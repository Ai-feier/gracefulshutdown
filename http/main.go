package main

import (
	"context"
	"github.com/Ai-feier/http/server"
	"log"
	"net/http"
	"time"
)

func main() {
	// 新建两个 server
	s1 := server.NewServer("service1", "localhost:8081")
	s1.Handle("/", func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("hello world"))
	})
	s2 := server.NewServer("service2", "localhost:8082")
	
	// 新建应用, 包含 service1, service2
	app := server.NewApp([]*server.Server{s1, s2}, server.WithShutDownCallBack(StoreCacheToDBCallBack))
	app.StartAndServer()
}

func StoreCacheToDBCallBack(ctx context.Context) {
	done := make(chan struct{}, 1)
	go func() {
		// 这里将 cache 中数据刷到 db
		log.Println("缓存刷新中...")
		time.Sleep(time.Second)
		done <- struct{}{}
	}()

	select {
	case <-done:
		log.Printf("缓存被刷新到了 DB")
	case <-ctx.Done():
		log.Printf("刷新缓存超时")
	}
}