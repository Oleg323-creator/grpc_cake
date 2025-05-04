package v3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

	// To check all possible pools.
	fees := []*big.Int{big.NewInt(500), big.NewInt(1000), big.NewInt(3000), big.NewInt(10000)}
	for _, fee := range fees {
		path := bytes.Join([][]byte{
			tokenIn.Bytes(),
			EncodeUint256(fee, 3, true),
			tokenOut.Bytes(),
		}, nil)

		logrus.Infof("Request parameters: TokenIn: %s, TokenOut: %s, Amount: %d, Slippage: %d, Chain: %s", tokenIn, tokenOut, req.Amount, req.SlippageBps, req.Chain)

		outAmount, err := v.quoterV2.QuoteExactInput(callOpts, path, amountIn)
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

	return nil, fmt.Errorf("failed to get quote: %v", err)
}

func (v *V3) ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (resp *quoteswap.ExecuteTxResponse, err error) {
	quote := req.QuotingResponse

	amountIn := new(big.Int)
	amountIn.SetString(quote.InAmount, 10)

	amountOut := new(big.Int)
	amountOut.SetString(quote.OutAmount, 10)

	tokenIn := common.HexToAddress(quote.InputToken)
	tokenOut := common.HexToAddress(quote.OutputToken)

	recipient := common.HexToAddress(os.Getenv("RECIPIENT_ADDR"))

	err = v.approveToken(ctx, tokenIn, recipient, v.routerAddress, amountIn)
	if err != nil {
		resp = &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error: &quoteswap.Error{
				Code:    5,
				Message: err.Error(),
			},
		}

		return resp, err
	}

	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	logrus.Infof("Preparing swap with parameters:\n TokenIn: %s;\n TokenOut: %s;\n AmountIn: %s;\n AmountOutMin: %s;\n Recipient: %s;\n Deadline: %s;\n",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), amountOut.String(), recipient.Hex(), deadline.String())

	var tx *types.Transaction
	var opts *bind.TransactOpts

	// To check all possible pools.
	fees := []*big.Int{big.NewInt(500), big.NewInt(1000), big.NewInt(3000), big.NewInt(10000)}
	for _, fee := range fees {
		params := routerV3.ISwapRouterExactInputSingleParams{
			TokenIn:           tokenIn,
			TokenOut:          tokenOut,
			Fee:               fee,
			Recipient:         recipient,
			Deadline:          deadline,
			AmountIn:          amountIn,
			AmountOutMinimum:  amountOut,
			SqrtPriceLimitX96: new(big.Int).SetBytes(make([]byte, 32)),
		}

		opts, err = v.opts(ctx)
		if err != nil {
			resp = &quoteswap.ExecuteTxResponse{
				Status: quoteswap.TransactionStatus_FAILED,
				Error: &quoteswap.Error{
					Code:    5,
					Message: fmt.Sprintf("getting opts failed: %s", err.Error()),
				},
			}

			return resp, err
		}

		tx, err = v.router.ExactInputSingle(opts, params)
		if err == nil {
			resp = &quoteswap.ExecuteTxResponse{
				TransactionHash: tx.Hash().Hex(),
				Status:          quoteswap.TransactionStatus_PENDING,
				SellTokenQty:    float64(amountIn.Int64()),
				ExecutedPrice:   float64(amountOut.Int64()),
			}

			return resp, nil
		}
		logrus.Warnf("Swap with fee %s failed: %v", fee.String(), err)
	}

	resp = &quoteswap.ExecuteTxResponse{
		Status: quoteswap.TransactionStatus_FAILED,
		Error: &quoteswap.Error{
			Code:    5,
			Message: fmt.Sprintf("swap failed: %s", err.Error()),
		},
	}

	return resp, err
}

func (v *V3) approveToken(ctx context.Context, tokenAddress, ownerAddress, spenderAddress common.Address, amount *big.Int) error {
	token, err := erc20.NewBlockchain(tokenAddress, v.client.Eth())
	if err != nil {
		return err
	}

	allowance, err := token.Allowance(&bind.CallOpts{Context: ctx}, ownerAddress, spenderAddress)
	if err != nil {
		return err
	}

	logrus.Infof("Allowance for %s to spend: %s", routerAddress.Hex(), allowance.String())

	opts, err := v.opts(ctx)
	if err != nil {
		return err
	}

	var tx *types.Transaction
	if allowance.Cmp(amount) < 0 {
		tx, err = token.Approve(opts, spenderAddress, amount)
		if err != nil {
			return err
		}

		for {
			receipt, err := v.client.Eth().TransactionReceipt(ctx, tx.Hash())
			if err == nil && receipt != nil {
				if receipt.Status == 1 {
					logrus.Info("approve tx was successful")
				} else {
					logrus.Info("approve tx failed")
				}
				break
			}
			time.Sleep(3 * time.Second)
		}
	}
	return nil
}

func (v *V3) opts(ctx context.Context) (*bind.TransactOpts, error) {
	chainID, err := v.client.Eth().ChainID(ctx)
	if err != nil {
		return nil, err
	}

	ptivateKey := os.Getenv("PRIVATE_KEY")
	privateKeyECDSA, err := crypto.HexToECDSA(strings.TrimPrefix(ptivateKey, "0x"))
	if err != nil {
		return nil, err
	}
	if privateKeyECDSA == nil {
		return nil, errors.New("privateKeyECDSA is nil")
	}

	auth := bind.NewKeyedTransactor(privateKeyECDSA, chainID)

	nonce, err := v.client.Eth().PendingNonceAt(ctx, auth.From)
	if err != nil {
		return nil, err
	}
	auth.Nonce = big.NewInt(int64(nonce))

	auth.GasTipCap, err = v.client.Eth().SuggestGasTipCap(ctx)
	if err != nil {
		return nil, err
	}

	header, err := v.client.Eth().HeaderByNumber(context.Background(), nil)
	if err != nil {
		return nil, err
	}
	auth.GasFeeCap = new(big.Int).Add(header.BaseFee, auth.GasTipCap)

	auth.Value = big.NewInt(0)
	auth.Context = ctx
	auth.GasLimit = 300000

	return auth, nil
}
