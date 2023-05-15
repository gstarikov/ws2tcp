package device

import (
	"context"
	"encoding/json"
	"github.com/gstarikov/ws2tcp/internal/api"
	"github.com/higebu/netfd"
	"go.uber.org/zap"
	"math/rand"
	"net"
	"strconv"
	"sync"
	"time"
)

/*
workflow:
- accept tcp connect
- read requestID
- endless respond with json each row:
--- requestID
--- random len string (700-1500 bytes)
--- crc64
*/

type Config struct {
	ListenAddr string        `default:"localhost" required:"true"`
	ListenPort uint16        `default:"1242" required:"true"`
	MinLen     uint16        `default:"750" required:"true"`
	MaxLen     uint16        `default:"1500" required:"true"`
	Interval   time.Duration `default:"10ms" required:"true" desc:"new packet interval"`
	Timeout    time.Duration `default:"5s" required:"true" desc:"tcp ack timeout, will terminate connection in case of timeout"`
}

type Device struct {
	listener      *net.TCPListener
	activeWorkers sync.WaitGroup
	log           *zap.Logger
	cfg           Config
}

func New(ctx context.Context, cfg Config, log *zap.Logger) (*Device, error) {
	rAddr, err := net.ResolveTCPAddr("tcp", net.JoinHostPort("", strconv.Itoa(int(cfg.ListenPort))))
	if err != nil {
		return nil, err
	}
	tListen, err := net.ListenTCP("tcp", rAddr)
	if err != nil {
		return nil, err
	}

	dev := &Device{
		listener: tListen,
		log:      log.Named("device").With(zap.String("listen", tListen.Addr().String())),
		cfg:      cfg,
	}
	dev.activeWorkers.Add(1)
	go dev.loop(ctx)
	return dev, nil
}

func (d *Device) WaitTillStop() {
	d.activeWorkers.Wait()
}

func (d *Device) loop(ctx context.Context) {
	defer func() {
		err := d.listener.Close()
		if err != nil {
			d.log.Error("cant close listener", zap.Error(err))
		} else {
			d.log.Info("listener closed successfully")
		}
		d.activeWorkers.Done()
	}()

	chConn := make(chan *net.TCPConn)
	defer close(chConn)

	d.activeWorkers.Add(1)
	go d.acceptConn(ctx, chConn)

	d.newConnLoop(ctx, chConn)
}

// graceful shutdown and new handling connection loop
func (d *Device) newConnLoop(ctx context.Context, chConn chan *net.TCPConn) {
	for {
		select {
		case <-ctx.Done():
			d.log.Info("context closed, stopping")
			return
		case conn := <-chConn:
			if conn == nil {
				continue
			}

			fd := netfd.GetFdFromConn(conn) // dont use conn.File() - in will create one more FD

			connLog := d.log.Named("conn").With(
				zap.String("lAddr", conn.LocalAddr().String()),
				zap.String("rAddr", conn.RemoteAddr().String()),
				zap.Int("fd", fd),
			)
			d.activeWorkers.Add(1)
			go d.worker(ctx, conn, connLog)
		}
	}
}

// accept connection loop
func (d *Device) acceptConn(ctx context.Context, chConn chan *net.TCPConn) {
	defer d.activeWorkers.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			conn, err := d.listener.AcceptTCP()
			if err != nil {
				d.log.Error("cant accept conn", zap.Error(err))
			} else {
				chConn <- conn
			}
		}
	}
}

func (d *Device) worker(ctx context.Context, conn *net.TCPConn, log *zap.Logger) {
	defer func() {
		if err := conn.SetLinger(0); err != nil {
			log.Warn("cant set linger = 0", zap.Error(err))
		}

		if err := conn.Close(); err != nil {
			log.Warn("conn closed with error", zap.Error(err))
		} else {
			log.Info("conn closed")
		}
	}()
	defer d.activeWorkers.Done()

	//tune up tcp
	if err := conn.SetKeepAlive(true); err != nil {
		log.Error("cant set keep-alive", zap.Error(err))
		return
	}
	if err := conn.SetNoDelay(true); err != nil {
		log.Error("cant set no delay", zap.Error(err))
		return
	}
	if err := conn.SetKeepAlivePeriod(time.Second); err != nil {
		log.Error("cant set keep-alive period", zap.Error(err))
		return
	}

	const deadLine = time.Second * 5

	readDeadline := time.Now().Add(deadLine)
	if err := conn.SetReadDeadline(readDeadline); err != nil {
		log.Error("cant set SetReadDeadline", zap.Error(err))
		return
	}

	//read request
	const bufSize = 1000
	rBuf := make([]byte, bufSize)
	read, err := conn.Read(rBuf)
	if err != nil {
		log.Error("cant read request", zap.Error(err))
		return
	}
	rBuf = rBuf[:read]
	log.Info("read request", zap.Int("bytes", read), zap.ByteString("buf", rBuf))

	//read requestID
	var request api.Request
	if err := json.Unmarshal(rBuf, &request); err != nil {
		log.Error("cant unmarchall request", zap.Error(err))
		return
	}

	log.Info("request parsed", zap.String("requestID", request.RequestID))

	ticker := time.NewTicker(d.cfg.Interval)
	defer ticker.Stop()

	logTicker := time.NewTicker(time.Second * 1)
	defer logTicker.Stop()

	//endless pushing loop
	var counter uint64
	var last uint64
	for {
		select {
		case <-ctx.Done():
			log.Info("context canceled")
			return
		case <-logTicker.C:
			delta := counter - last
			log.Debug("send in progress", zap.Uint64("counter", counter), zap.Uint64("delta", delta))
			last = counter
		case <-ticker.C:
			writeDeadLine := time.Now().Add(deadLine)
			if err := conn.SetWriteDeadline(writeDeadLine); err != nil {
				log.Error("cant set write deadline", zap.Error(err))
				return
			}
			//generate resp data
			counter++

			randInterval := d.cfg.MaxLen - d.cfg.MinLen
			randLen := rand.Int31n(int32(randInterval))
			randBuf := make([]byte, randLen)
			for i := int32(0); i < randLen; i++ {
				randBuf[i] = byte('0' + i%10)
			}

			//write resp
			resp := api.ResponseLine{
				RequestID: request.RequestID,
				Counter:   counter,
				Payload:   string(randBuf),
			}
			resp.CRC64 = resp.GetCRC64()

			jsonBuf, err := json.Marshal(resp)
			if err != nil {
				log.Error("cant marshal json", zap.Error(err))
				return
			}
			jsonBuf = append(jsonBuf, '\n')

			if _, err = conn.Write(jsonBuf); err != nil {
				log.Error("cant write to conn", zap.Error(err))
				return
			}
		}
	}
}
