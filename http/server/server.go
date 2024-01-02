package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/Ai-feier/sig"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"
)

type AppOption func(app *App)

// ShutdownCallBack App 关闭时的回调函数
// 默认超时 3s
// 用户可通过 context 自行控制
type ShutdownCallBack func(ctx context.Context)

func WithShutDownCallBack(cbs ...ShutdownCallBack) AppOption {
	return func(app *App) {
		app.cbs = cbs
	}
}

type App struct {
	servers []*Server

	// app 关闭的最长时间
	shutdownTimeout time.Duration

	// 留给 server 处理已有请求的时间
	waitTime time.Duration

	// 回调函数超时控制
	cbTime time.Duration

	cbs []ShutdownCallBack
}

func NewApp(servers []*Server, opts ...AppOption) *App {
	app := &App{
		servers:         servers,
		shutdownTimeout: time.Second * 30,
		waitTime:        time.Second * 10,
		cbTime:          3 * time.Second,
	}
	for _, opt := range opts {
		opt(app)
	}
	return app
}

func (app *App) StartAndServer() {
	// 启动服务
	for _, s := range app.servers {
		svc := s
		go func() {
			if err := svc.Start(); err != nil {
				if errors.Is(err, http.ErrServerClosed) {
					log.Printf("服务器%s已关闭", svc.name)
				} else {
					log.Printf("服务器%s异常退出", svc.name)
				}
			}
		}()
	}
	
	log.Println("应用启动成功")

	// app 启动成功
	// 监听退出信号
	// 监听两次信号, 第一次优雅终止, 第二次强行终止
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, sig.Signals...)
	<-ch
	fmt.Println("应用开始关闭...")
	go func() {
		select {
		case <-ch:
			log.Println("强制退出")
			os.Exit(1)
		case <-time.After(app.shutdownTimeout):
			log.Println("超时强行退出")
			os.Exit(1)
		}
	}()

	app.shutdown(context.Background())
}

func (app *App) shutdown(ctx context.Context) {
	log.Println("app start to shutdown")
	// 将 app 下所有 server 拒绝新请求
	for _, svc := range app.servers {
		svc.reject()
	}
	
	log.Println("等待已有请求处理")
	// 这里可以改造为实时统计正在处理的请求数量，为0 则下一步
	time.Sleep(time.Second * app.waitTime)
	
	log.Println("开始关闭应用")
	var wg sync.WaitGroup
	wg.Add(len(app.servers))
	for _, s := range app.servers {
		svc := s
		go func() {
			if err := svc.stop(ctx);err != nil {
				log.Println("服务器关闭失败", svc.name)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	
	// 执行回调函数
	wg.Add(len(app.cbs))
	log.Println("开始执行回调函数")
	for _, cb := range app.cbs {
		c := cb
		go func() {
			ctx2, cancal := context.WithCancel(ctx)
			c(ctx2)
			cancal()
			wg.Done()
		}()
	}
	wg.Wait()
	
	// 释放资源
	log.Println("开始释放资源")
	app.close()
}

func (app *App) close() {
	// 可补充释放资源逻辑
	time.Sleep(time.Second)
	log.Println("应用关闭")
}

// Server 对应一个服务
type Server struct {
	svc  *http.Server
	name string
	mux  *serverMux
}

// serverMux 请求锁
type serverMux struct {
	reject bool
	*http.ServeMux
}

func (s *serverMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.reject {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("服务器已关闭"))
		return
	}
	s.ServeMux.ServeHTTP(w, r)
}

func NewServer(name, addr string) *Server {
	mux := &serverMux{ServeMux: http.NewServeMux()}
	return &Server{
		name: name,
		mux:  mux,
		svc: &http.Server{
			Handler: mux,
			Addr:    addr,
		},
	}
}

func (s *Server) Start() error {
	return s.svc.ListenAndServe()
}

func (s *Server) Handle(pattern string, handler http.HandlerFunc) {
	s.mux.Handle(pattern, handler)
}

func (s *Server) reject() {
	s.mux.reject = true
}

func (s *Server) stop(ctx context.Context) error {
	log.Println("server: ", s.name, "关闭中")
	return s.svc.Shutdown(ctx)
}
