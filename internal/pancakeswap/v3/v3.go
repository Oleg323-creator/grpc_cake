package v3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/sirupsen/logrus"
	"grpc_cake/gen/go/quoteswap"
	"grpc_cake/internal/blockchain"
	erc20 "grpc_cake/internal/blockchain/abi/gen/erc20"
	"grpc_cake/internal/blockchain/abi/gen/quoterV2"
	"grpc_cake/internal/blockchain/abi/gen/routerV3"
)

var (
	routerAddress   = common.HexToAddress("0x1b81D678ffb9C0263b24A97847620C99d213eB14")
	quoterV2Address = common.HexToAddress("0xB048Bbc1Ee6b733FFfCFb9e9CeF7375518e25997")
)

type V3 struct {
	router          *routerV3.Blockchain
	routerAddress   common.Address
	quoterV2        *quoterV2.QuoterV2Caller
	quoterV2Address common.Address
	client          *blockchain.Client
}

func NewV3(client *blockchain.Client) (*V3, error) {
	router, err := routerV3.NewBlockchain(routerAddress, client.Eth())
	if err != nil {
		return nil, err
	}

	quoterV2, err := quoterV2.NewQuoterV2Caller(quoterV2Address, client.Eth())
	if err != nil {
		return nil, err
	}

	return &V3{
		router:          router,
		routerAddress:   routerAddress,
		quoterV2:        quoterV2,
		quoterV2Address: quoterV2Address,
		client:          client,
	}, nil
}

func (v *V3) GetQuote(ctx context.Context, req *quoteswap.GetQuoteRequest) (resp *quoteswap.GetQuoteResponse, err error) {
	tokenIn := common.HexToAddress(req.TokenIn)
	tokenOut := common.HexToAddress(req.TokenOut)
	amountIn := new(big.Int).SetUint64(req.Amount)
	callOpts := &bind.CallOpts{Context: ctx}
	//path := createPath(tokenIn, tokenOut)

	// To check all possible pools.
	fees := []*big.Int{big.NewInt(500), big.NewInt(1000), big.NewInt(3000), big.NewInt(10000)}
	for _, fee := range fees {
		path := bytes.Join([][]byte{
			tokenIn.Bytes(),
			EncodeUint256(fee, 3, true),
			tokenOut.Bytes(),
		}, nil)

		outAmount, err := v.quoterV2.QuoteExactInput(callOpts, path, amountIn)

		logrus.Infof("Request parameters: TokenIn: %s, TokenOut: %s, Amount: %d, Slippage: %d, Chain: %s", tokenIn, tokenOut, req.Amount, req.SlippageBps, req.Chain)
		if err == nil {
			resp = &quoteswap.GetQuoteResponse{
				InputToken:  req.TokenIn,
				InAmount:    amountIn.String(),
				OutputToken: req.TokenOut,
				OutAmount:   outAmount.String(),
				SlippageBps: int32(req.SlippageBps),
				Dex:         req.Dex,
				Chain:       v.client.Chain,
			}
			return resp, nil
		}
	}

	return nil, errors.New(fmt.Sprintf("failed to get quote: %v", err))
}

func (v *V3) ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (resp *quoteswap.ExecuteTxResponse, err error) {
	quote := req.QuotingResponse
	amountIn := new(big.Int)
	amountIn.SetString(quote.InAmount, 10)

	tokenIn := common.HexToAddress(quote.InputToken)
	tokenOut := common.HexToAddress(quote.OutputToken)
	to := common.HexToAddress(req.RecipientAddress)

	log.Printf("ExecuteSwap request: InAmount: %s, InputToken: %s, OutputToken: %s, Recipient: %s",
		quote.InAmount, quote.InputToken, quote.OutputToken, req.RecipientAddress)

	err = v.approveToken(ctx, tokenIn, to, v.routerAddress, amountIn)
	if err != nil {
		resp = &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error: &quoteswap.Error{
				Code:    1,
				Message: fmt.Sprintf("approval failed: %v", err),
			},
		}

		return resp, err
	}

	amountOutMin := big.NewInt(0)
	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	log.Printf("amountIn: %s", amountIn.String())
	log.Printf("amountOutMin: %s", amountOutMin.String())
	log.Printf("to: %s", to)

	log.Printf("Calculated amountOutMin: %s, deadline: %s", amountOutMin.String(), deadline.String())

	log.Printf("Preparing swap with parameters: TokenIn: %s, TokenOut: %s, AmountIn: %s, AmountOutMin: %s, Recipient: %s, Deadline: %s",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), amountOutMin.String(), to.Hex(), deadline.String())
	var tx *types.Transaction
	// To check all possible pools.
	fees := []*big.Int{big.NewInt(500), big.NewInt(1000), big.NewInt(3000), big.NewInt(10000)}
	for _, fee := range fees {
		params := routerV3.ISwapRouterExactInputSingleParams{
			TokenIn:           tokenIn,
			TokenOut:          tokenOut,
			Fee:               fee,
			Recipient:         to,
			Deadline:          deadline,
			AmountIn:          amountIn,
			AmountOutMinimum:  amountOutMin,
			SqrtPriceLimitX96: new(big.Int).SetBytes(make([]byte, 32)),
		}
		log.Printf("Params: TokenIn: %s, TokenOut: %s, Fee: %s, Recipient: %s, Deadline: %s, AmountIn: %s, AmountOutMin: %s, SqrtPriceLimitX96: %s",
			tokenIn.Hex(), tokenOut.Hex(), fee.String(), to.Hex(), deadline.String(), amountIn.String(), amountOutMin.String(), "0")

		chainID, err := v.client.Eth().ChainID(ctx)
		if err != nil {
			logrus.Error(err)
			continue
		}

		ptivateKey := os.Getenv("PRIVATE_KEY")
		privateKeyECDSA, err := crypto.HexToECDSA(strings.TrimPrefix(ptivateKey, "0x"))
		if err != nil {
			logrus.Error("!!!", err)
			continue
		}
		if privateKeyECDSA == nil {
			log.Fatal("privateKeyECDSA is nil")
		}

		auth := bind.NewKeyedTransactor(privateKeyECDSA, chainID)

		nonce, err := v.client.Eth().PendingNonceAt(ctx, auth.From)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch nonce: %w", err)
		}
		auth.Nonce = big.NewInt(int64(nonce))

		//currentBlock, err := v.client.Eth().BlockByNumber(ctx, nil)
		//if err != nil {
		//	return nil, fmt.Errorf("failed to fetch block: %w", err)
		//}

		auth.GasTipCap, err = v.client.Eth().SuggestGasTipCap(ctx) //big.NewInt(4000000000) // 4 gwei
		if err != nil {
			logrus.Error("!!!", err)
			continue
		}

		//baseFee := currentBlock.BaseFee()
		block, err := v.client.Eth().BlockByNumber(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("block fetch error: %w", err)
		}
		auth.GasFeeCap = new(big.Int).Add(block.BaseFee(), auth.GasTipCap)

		auth.Value = big.NewInt(0)
		auth.Context = ctx
		auth.Value = nil
		auth.GasLimit = 300000

		tx, err = v.router.ExactInputSingle(auth, params)
		if err == nil {
			resp = &quoteswap.ExecuteTxResponse{
				TransactionHash: tx.Hash().Hex(),
				Status:          quoteswap.TransactionStatus_PENDING,
				SellTokenQty:    float64(amountIn.Int64()),
				ExecutedPrice:   float64(amountIn.Int64()),
			}

			return resp, nil
		}
		log.Printf("Swap with fee %s failed: %v", fee.String(), err)
	}

	resp = &quoteswap.ExecuteTxResponse{
		Status: quoteswap.TransactionStatus_FAILED,
		Error: &quoteswap.Error{
			Code:    5, //not found
			Message: fmt.Sprintf("swap failed: %v", err),
		},
	}

	return resp, err
}

func (v *V3) approveToken(ctx context.Context, token, owner, spender common.Address, amount *big.Int) error {
	instance, err := erc20.NewBlockchain(token, v.client.Eth())
	if err != nil {
		return err
	}

	allowance, err := instance.Allowance(&bind.CallOpts{Context: ctx}, owner, spender)
	if err != nil {
		return errors.New(fmt.Sprintf("checking allowance failed: %v", err))
	}
	log.Printf("Allowance for %s to spend: %s", routerAddress.Hex(), allowance.String())

	chainID, err := v.client.Eth().ChainID(ctx)
	if err != nil {
		return err
	}

	ptivateKey := os.Getenv("PRIVATE_KEY")
	privateKeyECDSA, err := crypto.HexToECDSA(strings.TrimPrefix(ptivateKey, "0x"))
	if err != nil {
		return err
	}
	if privateKeyECDSA == nil {
		log.Fatal("privateKeyECDSA is nil")
	}

	auth := bind.NewKeyedTransactor(privateKeyECDSA, chainID)

	nonce, err := v.client.Eth().PendingNonceAt(ctx, auth.From)
	if err != nil {
		return err
	}
	auth.Nonce = big.NewInt(int64(nonce))

	//currentBlock, err := v.client.Eth().BlockByNumber(ctx, nil)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to fetch block: %w", err)
	//}

	auth.GasTipCap, err = v.client.Eth().SuggestGasTipCap(ctx) //big.NewInt(4000000000) // 4 gwei
	if err != nil {
		return err
	}

	//baseFee := currentBlock.BaseFee()
	block, err := v.client.Eth().BlockByNumber(ctx, nil)
	if err != nil {
		return err
	}
	auth.GasFeeCap = new(big.Int).Add(block.BaseFee(), auth.GasTipCap)

	auth.Value = big.NewInt(0)
	auth.Context = ctx
	auth.Value = nil
	auth.GasLimit = 300000

	if allowance.Cmp(amount) < 0 {
		tx, err := instance.Approve(auth, spender, amount)
		if err != nil {
			return errors.New(fmt.Sprintf("token approval failed: %v", err))
		}

		for {
			receipt, err := v.client.Eth().TransactionReceipt(ctx, tx.Hash())
			if err == nil && receipt != nil {
				break
			}
			time.Sleep(3 * time.Second)
		}
	}
	return nil
}
