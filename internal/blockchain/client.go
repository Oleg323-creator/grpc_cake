package blockchain

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/ethereum/go-ethereum/ethclient"
)

const (
	ChainBSC  = "bsc"
	ChainETH  = "eth"
	ChainBase = "base"
)

var rpcURLs = map[string]string{
	ChainBSC:  "https://bsc-mainnet.infura.io/v3/",
	ChainETH:  "https://mainnet.infura.io/v3/",
	ChainBase: "https://base-mainnet.infura.io/v3/",
}

type Client struct {
	client *ethclient.Client
	Chain  string
}

func (c *Client) Eth() *ethclient.Client {
	return c.client
}

func NewClient(chain string) (*Client, error) {
	godotenv.Load()

	rpcURL, ok := rpcURLs[chain]
	if !ok {
		return nil, errors.New(fmt.Sprintf("unsupported chain: %s", chain))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rpcURL = fmt.Sprintf("%s%s", rpcURL, os.Getenv("API_KEY"))
	rawClient, err := ethclient.DialContext(ctx, rpcURL)

	if err != nil {
		return nil, err
	}

	return &Client{
		client: rawClient,
		Chain:  chain,
	}, nil
}
