package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"phira-mp/server"
)

func main() {
	// 解析命令行参数
	port := flag.Int("port", 12346, "Server port")
	flag.Parse()

	// 加载配置
	config, err := server.LoadConfig("server_config.yml")
	if err != nil {
		log.Printf("Failed to load config: %v, using default", err)
		config = server.DefaultConfig()
	}

	// 创建服务器
	srv := server.NewServer(config)

	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动服务器
	go func() {
		address := fmt.Sprintf(":%d", *port)
		if err := srv.Start(address); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("Phira-MP Server started on port %d", *port)
	log.Println("Press Ctrl+C to stop")

	// 等待信号
	<-sigChan
	log.Println("Shutting down...")
	srv.Stop()
	log.Println("Server stopped")
}
