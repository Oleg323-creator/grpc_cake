package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"grpc_cake/gen/go/quoteswap"
	"grpc_cake/internal/blockchain"
	"grpc_cake/internal/pancakeswap"
	v2 "grpc_cake/internal/pancakeswap/v2"
	v3 "grpc_cake/internal/pancakeswap/v3"
	"grpc_cake/internal/service"
)

func main() {
	s := grpc.NewServer()

	v2Services := make(map[string]pancakeswap.Swapper)
	v3Services := make(map[string]pancakeswap.Swapper)

	clients := make(map[string]*blockchain.Client)

	for _, chain := range []string{blockchain.ChainBSC, blockchain.ChainETH, blockchain.ChainBase} {
		client, err := blockchain.NewClient(chain)
		if err != nil {
			logrus.Errorf("failed to create client for chain %s: %v", chain, err)
			continue
		}
		clients[chain] = client

		v2Service, err := v2.NewV2(client)
		if err != nil {
			log.Fatalf("failed to create V2 service for %s: %v", chain, err)
		}
		v2Services[chain] = v2Service

		v3Service, err := v3.NewV3(client)
		if err != nil {
			logrus.Errorf("failed to create V3 service for %s: %v", chain, err)
		}
		v3Services[chain] = v3Service
	}

	srv := &service.QuoteSwapServiceServer{
		V2Services: v2Services,
		V3Services: v3Services,
	}

	quoteswap.RegisterQuoteSwapServiceServer(s, srv)

	reflection.Register(s)

	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logrus.Fatalf("failed to listen: %v", err)
	}

	logrus.Info("Starting QuoteSwap service...")

	go func() {
		if err := s.Serve(listener); err != nil {
			logrus.Errorf("failed to serve: %v", err)
		}
	}()

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-stopCh

	logrus.Info("Shutting down QuoteSwap service...")
	s.GracefulStop()
	logrus.Info("Service has been stopped gracefully.")
}
