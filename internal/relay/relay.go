package relay

import (
	"context"
	"errors"
	"github.com/fasthttp/websocket"
	"github.com/gofrs/uuid"
	"github.com/higebu/netfd"
	"github.com/valyala/fasthttp"
	"go.uber.org/zap"
	"net"
	"sync"
	"time"
)

type Config struct {
	Listen          string        `default:"localhost:1337" required:"true"`
	Device          string        `default:"localhost:1242" required:"true"`
	ReadTimeout     time.Duration `default:"5s" required:"true"`
	WriteTimeout    time.Duration `default:"5s" required:"true"`
	IdleTimeout     time.Duration `default:"15s" required:"true"`
	ReadBufferSize  int           `default:"1000" required:"true"`
	WriteBufferSize int           `default:"1000" required:"true"`
}

type Relay struct {
	cfg           Config
	devAddr       *net.TCPAddr
	srv           *fasthttp.Server
	ls            net.Listener
	activeWorkers sync.WaitGroup
	log           *zap.Logger
	wsUpgrader    websocket.FastHTTPUpgrader
	chStop        chan struct{}
}

func New(cfg Config, log *zap.Logger) (*Relay, error) {

	devAddr, err := net.ResolveTCPAddr("tcp", cfg.Device)
	if err != nil {
		return nil, err
	}

	rel := &Relay{
		cfg:           cfg,
		activeWorkers: sync.WaitGroup{},
		log:           log,
		devAddr:       devAddr,
		chStop:        make(chan struct{}),
		wsUpgrader: websocket.FastHTTPUpgrader{
			HandshakeTimeout: cfg.ReadTimeout,
			ReadBufferSize:   cfg.ReadBufferSize,
			WriteBufferSize:  cfg.WriteBufferSize,
			CheckOrigin:      func(ctx *fasthttp.RequestCtx) bool { return true },
		},
	}

	srv := &fasthttp.Server{
		Name:            "ws2tcp",
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		ReadTimeout:     cfg.ReadTimeout,
		WriteTimeout:    cfg.WriteTimeout,
		IdleTimeout:     cfg.IdleTimeout,
		CloseOnShutdown: true,
		Handler:         rel.handler,
	}

	ls, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, err
	}
	rel.srv = srv
	rel.ls = ls

	rel.activeWorkers.Add(1)
	go rel.serve()

	return rel, nil
}

func (r *Relay) serve() {
	defer r.activeWorkers.Done()
	err := r.srv.Serve(r.ls)
	if err != nil {
		r.log.Error("server error", zap.Error(err))
	} else {
		r.log.Info("server stopped")
	}
}

func (r *Relay) WaitTillStop() {
	close(r.chStop)
	err := r.srv.ShutdownWithContext(context.Background())
	if err != nil {
		r.log.Error("close error", zap.Error(err))
	} else {
		r.log.Info("close stopped")
	}
	r.log.Info("wait for workers")
	r.activeWorkers.Wait()
	r.log.Info("all workers are done")
}

func (r *Relay) handler(ctx *fasthttp.RequestCtx) {
	log := r.log.With(
		zap.String("localAddr", ctx.LocalAddr().String()),
		zap.String("remoteAddr", ctx.RemoteAddr().String()),
		zap.Int("fd", netfd.GetFdFromConn(ctx.Conn())),
	)
	log.Info("new request")

	requestID, err := uuid.NewV4()
	if err != nil {
		log.Error("cant create uuid", zap.Error(err))
		ctx.Error("cant create uuid", fasthttp.StatusInternalServerError)
		return
	}

	devConn, err := newDeviceConn(requestID.String(), r.devAddr, deviceConfig{
		ReadTimeout:  r.cfg.ReadTimeout,
		WriteTimeout: r.cfg.WriteTimeout,
		Log:          log,
	})
	if err != nil {
		log.Warn("cant connect to dev", zap.Error(err))
		ctx.Error("cant connect to device", fasthttp.StatusBadGateway)
		return
	}
	log.Info("connected")

	r.activeWorkers.Add(1)
	err = r.wsUpgrader.Upgrade(ctx, func(ws *websocket.Conn) {
		defer r.activeWorkers.Done()
		defer func() {
			log.Info("shutdown... closing device connection")
			devConn.Close()
		}()

		log.Info("upgrade to ws done")
		defer func() {
			err := errors.Join(
				ws.WriteMessage(websocket.TextMessage, []byte("going shutdown")),
				ws.Close(),
			)
			if err != nil {
				log.Error("cant close ws", zap.Error(err))
			} else {
				log.Info("ws closed")
			}
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		var counter uint64
		var last uint64

		for {
			//ToDo: there is no graceful shutdown
			select {
			case <-r.chStop:
				log.Info("graceful shutdown ws")
				return
			case <-ticker.C:
				delta := counter - last
				last = counter
				log.Info("processed",
					zap.Uint64("count", counter),
					zap.Uint64("delta", delta),
				)
			default:
				msg, err := devConn.GetLine(r.chStop)
				if err != nil {
					log.Warn("device error", zap.Error(err))
					return
				}

				if crc64 := msg.GetCRC64(); msg.CRC64 != crc64 {
					log.Error("broken message",
						zap.Uint64("want", msg.CRC64),
						zap.Uint64("got", crc64),
						zap.Any("msg", msg),
					)
					return
				}

				err = ws.WriteJSON(msg)
				if err != nil {
					log.Warn("cant write to ws", zap.Error(err))
					return
				}
				counter++
			}
		}
	})

	if err != nil {
		r.activeWorkers.Done() //Add noy here to avoid race
		if _, ok := err.(websocket.HandshakeError); ok {
			log.Error("websocket error", zap.Error(err))
		}
		return
	}
}
