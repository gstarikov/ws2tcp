package relay

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gstarikov/ws2tcp/internal/api"
	"github.com/higebu/netfd"
	"go.uber.org/zap"
	"io"
	"net"
	"time"
)

const bufSize = 1000

type deviceConfig struct {
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Log          *zap.Logger
}

type device struct {
	conn *net.TCPConn
	cfg  deviceConfig
	acc  *accumulator
}

func newDeviceConn(requestID string, endpoint *net.TCPAddr, cfg deviceConfig) (*device, error) {
	log := cfg.Log.Named("device")
	conn, err := net.DialTCP("tcp", nil, endpoint)
	if err != nil {
		log.Error("cant dial", zap.Error(err))
		return nil, err
	}
	log = log.With(
		zap.String("deviceRemote", conn.RemoteAddr().String()),
		zap.String("deviceLocal", conn.LocalAddr().String()),
		zap.Int("deviceFD", netfd.GetFdFromConn(conn)),
	)
	log.Info("connected")
	defer func() {
		if err == nil {
			log.Info("no error")
			return
		}
		log.Error("error on init, close conn", zap.Error(err))
		if err := conn.Close(); err != nil {
			log.Error("error on close", zap.Error(err))
		}
	}()

	if err := conn.SetLinger(0); err != nil {
		log.Error("cant set longer", zap.Error(err))
		return nil, err
	}
	//tune up tcp
	if err := conn.SetKeepAlive(true); err != nil {
		log.Error("cant set keep-alive", zap.Error(err))
		return nil, err
	}
	if err := conn.SetNoDelay(true); err != nil {
		log.Error("cant set no delay", zap.Error(err))
		return nil, err
	}
	if err := conn.SetKeepAlivePeriod(time.Second); err != nil {
		log.Error("cant set keep-alive period", zap.Error(err))
		return nil, err
	}

	if err := conn.SetReadBuffer(bufSize); err != nil {
		log.Error("cant SetReadBuffer", zap.Error(err))
		return nil, err
	}
	if err := conn.SetWriteBuffer(bufSize); err != nil {
		log.Error("cant SetReadBuffer", zap.Error(err))
		return nil, err
	}
	deadLine := time.Now().Add(cfg.WriteTimeout)
	if err := conn.SetWriteDeadline(deadLine); err != nil {
		log.Error("cant SetWriteDeadline", zap.Error(err))
		return nil, err
	}

	req := api.Request{RequestID: requestID}
	reqJson, err := json.Marshal(&req)
	if err != nil {
		log.Error("cant marshall request", zap.Error(err))
		return nil, err
	}
	n, err := conn.Write(reqJson)
	if err != nil {
		log.Error("cant send request", zap.Error(err))
		return nil, err
	}
	log.Info("request send", zap.Int("write", n))

	cfg.Log = log // override log
	return &device{
		conn: conn,
		cfg:  cfg,
		acc:  newAccumulator(bufSize),
	}, nil
}

func (d *device) Close() {
	if err := d.conn.Close(); err != nil {
		d.cfg.Log.Warn("cant close device connect", zap.Error(err))
	}
	d.cfg.Log.Info("device connect closed")
}

// GetLine mission - read until full json row will be read
func (d *device) GetLine(stop <-chan struct{}) (*api.ResponseLine, error) {
	socketNotClosed := true
	for socketNotClosed {
		select {
		case <-stop:
			return nil, context.Canceled
		default:

		}

		deadLine := time.Now().Add(d.cfg.ReadTimeout)
		if err := d.conn.SetReadDeadline(deadLine); err != nil {
			d.cfg.Log.Error("cant SetReadDeadline", zap.Error(err))
			return nil, err
		}

		rBuf := make([]byte, bufSize)
		n, err := d.conn.Read(rBuf)
		rBuf = rBuf[:n]

		if err != nil {
			if errors.Is(err, io.EOF) && n != 0 {
				socketNotClosed = false
			} else {
				return nil, err
			}
		}

		rLine, readMore, err := d.acc.add(rBuf)
		if readMore {
			d.cfg.Log.Warn("fragmented response, read more",
				zap.ByteString("current buffer", d.acc.buf))
			continue
		}
		if err != nil {
			d.cfg.Log.Warn("json error",
				zap.ByteString("accumulator", d.acc.buf),
				zap.ByteString("buf", rBuf),
			)
		}
		return rLine, err
	}
	//only one case - we cannot read anything line json
	return nil, io.EOF
}
