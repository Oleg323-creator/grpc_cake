package v2

import (
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
	"grpc_cake/internal/blockchain/abi/gen/routerV2"
)

var RouterAddresses = map[string]common.Address{
	"eth":  common.HexToAddress("0x10ED43C718714eb63d5aA57B78B54704E256024E"), // bsc router addr
	"bsc":  common.HexToAddress("0xEfF92A263d31888d860bD50809A8D171709b7b1c"), // eth router addr
	"base": common.HexToAddress("0x8cFe327CEc66d1C090Dd72bd0FF11d690C33a2Eb"), // base router addr
}

type V2 struct {
	router        *routerV2.Blockchain
	routerAddress common.Address
	client        *blockchain.Client
}

func NewV2(client *blockchain.Client) (*V2, error) {
	routerAddress, ok := RouterAddresses[client.Chain]
	if !ok {
		return nil, errors.New(fmt.Sprintf("router address not found for chain: %s", client.Chain))
	}

	router, err := routerV2.NewBlockchain(routerAddress, client.Eth())
	if err != nil {
		return nil, err
	}

	return &V2{
		router:        router,
		routerAddress: routerAddress,
		client:        client,
	}, nil
}

func (v *V2) GetQuote(ctx context.Context, req *quoteswap.GetQuoteRequest) (resp *quoteswap.GetQuoteResponse, err error) {
	amountIn := new(big.Int).SetUint64(req.Amount)
	path := []common.Address{common.HexToAddress(req.TokenIn), common.HexToAddress(req.TokenOut)}

	logrus.Infof("Request parameters: TokenIn: %s, TokenOut: %s, Amount: %d, Slippage: %d, Chain: %s", req.TokenIn, req.TokenOut, req.Amount, req.SlippageBps, req.Chain)

	amountsOut, err := v.router.GetAmountsOut(&bind.CallOpts{Context: ctx}, amountIn, path)
	if err != nil {
		return nil, err
	}

	resp = &quoteswap.GetQuoteResponse{
		InputToken:  req.TokenIn,
		InAmount:    amountIn.String(),
		OutputToken: req.TokenOut,
		OutAmount:   amountsOut[len(amountsOut)-1].String(),
		SlippageBps: int32(req.SlippageBps),
		Dex:         req.Dex,
		Chain:       v.client.Chain,
	}

	return resp, err
}

func (v *V2) ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (resp *quoteswap.ExecuteTxResponse, err error) {
	quote := req.QuotingResponse

	amountIn := new(big.Int)
	if _, ok := amountIn.SetString(quote.InAmount, 10); !ok {
		return nil, errors.New("invalid amount in value")
	}

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
				Code:    1,
				Message: fmt.Sprintf("approval failed: %s", err.Error()),
			},
		}

		return resp, err
	}

	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	logrus.Infof("Preparing swap with parameters:\n TokenIn: %s;\n TokenOut: %s;\n AmountIn: %s;\n AmountOutMin: %s;\n Recipient: %s;\n Deadline: %s;\n",
		tokenIn.Hex(), tokenOut.Hex(), amountIn.String(), amountOut.String(), recipient.Hex(), deadline.String())

	var tx *types.Transaction
	opts, err := v.opts(ctx)
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

	tx, err = v.router.SwapExactTokensForTokens(
		opts,
		amountIn,
		amountOut,
		[]common.Address{tokenIn, tokenOut},
		recipient,
		deadline,
	)
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	resp = &quoteswap.ExecuteTxResponse{
		TransactionHash: tx.Hash().Hex(),
		Status:          quoteswap.TransactionStatus_PENDING,
		SellTokenQty:    float64(amountIn.Int64()),
		ExecutedPrice:   float64(amountOut.Int64()),
	}

	return resp, nil
}

func (v *V2) approveToken(ctx context.Context, tokenAddress, ownerAddress, spenderAddress common.Address, amount *big.Int) error {
	token, err := erc20.NewBlockchain(tokenAddress, v.client.Eth())
	if err != nil {
		return err
	}

	allowance, err := token.Allowance(&bind.CallOpts{Context: ctx}, ownerAddress, spenderAddress)
	if err != nil {
		return err
	}

	logrus.Infof("Allowance for %s to spend: %s", v.routerAddress.Hex(), allowance.String())

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

func (v *V2) opts(ctx context.Context) (*bind.TransactOpts, error) {
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
