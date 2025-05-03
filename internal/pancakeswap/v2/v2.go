package v2

import (
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

func (v *V2) ExecuteSwap(ctx context.Context, req *quoteswap.ExecuteTxRequest) (*quoteswap.ExecuteTxResponse, error) {
	quote := req.QuotingResponse

	amountIn := new(big.Int)
	if _, ok := amountIn.SetString(quote.InAmount, 10); !ok {
		return nil, errors.New("invalid in_amount value")
	}

	amountOutMin := new(big.Int)
	if _, ok := amountOutMin.SetString(quote.OutAmount, 10); !ok {
		return nil, errors.New("invalid out_amount value")
	}

	fromToken := common.HexToAddress(quote.InputToken)
	toToken := common.HexToAddress(quote.OutputToken)
	recipient := common.HexToAddress(req.RecipientAddress)

	if err := v.approveToken(ctx, fromToken, recipient, v.routerAddress, amountIn); err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	//amountOutMin := new(big.Int).Mul(amountOutMin, big.NewInt(int64(10000-quote.SlippageBps)))
	//amountOutMin.Div(amountOutMin, big.NewInt(10000))
	//fmt.Printf("Slippage BPS: %d, Amount Out: %s, Amount Out Min: %s\n", quote.SlippageBps, amountOutMin.String(), amountOutMin.String())

	//fmt.Println(1)
	//amountOutmin, err := v.router.GetAmountsOut(&bind.CallOpts{Context: ctx}, amountIn, []common.Address{fromToken, toToken})
	//if err != nil {
	//	return &quoteswap.ExecuteTxResponse{
	//		Status: quoteswap.TransactionStatus_FAILED,
	//		Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
	//	}, nil
	//}
	//fmt.Printf("GetAmountsOut result: %v\n", amountOutmin)
	//fmt.Printf("Calculated amounts out: %s\n", amountOutmin[len(amountOutmin)-1].String())
	//fmt.Println(2)
	//fmt.Println("Trying to execute swap with the following values:")
	//fmt.Printf("Amount In: %s\n", amountIn.String())
	//
	//fmt.Printf("From Token: %s\n", fromToken.Hex())
	//fmt.Printf("To Token: %s\n", toToken.Hex())
	//if len(amountOutmin) < 2 || amountOutmin[len(amountOutmin)-1].Cmp(big.NewInt(0)) == 0 {
	//	return &quoteswap.ExecuteTxResponse{
	//		Status: quoteswap.TransactionStatus_FAILED,
	//		Error:  &quoteswap.Error{Code: 5, Message: "Insufficient liquidity for swap."},
	//	}, nil
	//}
	//fmt.Println(3)
	//amountOutMin := new(big.Int).Mul(amountOutmin[len(amountOutmin)-1], big.NewInt(int64(10000-quote.SlippageBps)))
	//amountOutMin.Div(amountOutMin, big.NewInt(10000))
	//fmt.Printf("Amount Out Min: %s\n", amountOutMin.String())
	//fmt.Printf("Slippage BPS: %d, Amount Out: %s, Amount Out Min: %s\n", quote.SlippageBps, amountOutMin.String(), amountOutMin.String())
	//
	//fmt.Printf("Calculated amounts out: %s\n", amountOutMin)

	deadline := big.NewInt(time.Now().Add(10 * time.Minute).Unix())

	chainID, err := v.client.Eth().ChainID(ctx)
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}
	fmt.Println(4)
	ptivateKey := os.Getenv("PRIVATE_KEY")
	privateKeyECDSA, err := crypto.HexToECDSA(strings.TrimPrefix(ptivateKey, "0x"))
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	if privateKeyECDSA == nil {
		log.Fatal("privateKeyECDSA is nil")
	}

	auth := bind.NewKeyedTransactor(privateKeyECDSA, chainID)

	nonce, err := v.client.Eth().PendingNonceAt(ctx, auth.From)
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}
	auth.Nonce = big.NewInt(int64(nonce))

	//currentBlock, err := v.client.Eth().BlockByNumber(ctx, nil)
	//if err != nil {
	//	return nil, fmt.Errorf("failed to fetch block: %w", err)
	//}

	auth.GasTipCap, err = v.client.Eth().SuggestGasTipCap(ctx) //big.NewInt(4000000000) // 4 gwei
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	//baseFee := currentBlock.BaseFee()
	header, err := v.client.Eth().HeaderByNumber(context.Background(), nil) // nil для последнего блока
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	auth.GasFeeCap = new(big.Int).Add(header.BaseFee, auth.GasTipCap)

	auth.Value = big.NewInt(0)
	auth.Context = ctx
	auth.Value = nil
	auth.GasLimit = 300000

	tx, err := v.router.SwapExactTokensForTokens(
		auth,
		amountIn,
		amountOutMin,
		[]common.Address{fromToken, toToken},
		recipient,
		deadline,
	)
	if err != nil {
		return &quoteswap.ExecuteTxResponse{
			Status: quoteswap.TransactionStatus_FAILED,
			Error:  &quoteswap.Error{Code: 5, Message: err.Error()},
		}, nil
	}

	//executedPrice := calculateExecutedPrice(amountIn, amountOutMin)

	return &quoteswap.ExecuteTxResponse{
		TransactionHash: tx.Hash().Hex(),
		Status:          quoteswap.TransactionStatus_PENDING,
		SellTokenQty:    float64(amountIn.Int64()),
		ExecutedPrice:   float64(amountIn.Int64()),
	}, nil
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

	auth.GasTipCap, err = v.client.Eth().SuggestGasTipCap(ctx)
	if err != nil {
		return err
	}

	header, err := v.client.Eth().HeaderByNumber(context.Background(), nil) // nil для последнего блока
	if err != nil {
		log.Fatalf("Не удалось получить заголовок блока: %v", err)
	}

	auth.GasFeeCap = new(big.Int).Add(header.BaseFee, auth.GasTipCap)

	auth.Value = big.NewInt(0)
	auth.Context = ctx
	auth.Value = nil
	auth.GasLimit = 300000

	if allowance.Cmp(amount) < 0 {
		tx, err := token.Approve(auth, spenderAddress, amount)
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

func calculateAmountOutMin(amountOut *big.Int, slippageBps uint32) *big.Int {
	slippage := new(big.Int).Mul(amountOut, big.NewInt(int64(slippageBps)))
	slippage.Div(slippage, big.NewInt(10000)) //basic (100 = 1%)
	return new(big.Int).Sub(amountOut, slippage)
}

func calculateExecutedPrice(amountIn, amountOut *big.Int) float64 {
	if amountOut.Sign() == 0 {
		return 0.0
	}
	price := new(big.Float).Quo(new(big.Float).SetInt(amountOut), new(big.Float).SetInt(amountIn))
	result, _ := price.Float64()
	return result
}

func (v *V2) prepareTx(ctx context.Context) (*bind.TransactOpts, error) {
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
		log.Fatal("privateKeyECDSA is nil")
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

	//baseFee := currentBlock.BaseFee()
	block, err := v.client.Eth().BlockByNumber(ctx, nil)
	if err != nil {
		return nil, err
	}
	auth.GasFeeCap = new(big.Int).Add(block.BaseFee(), auth.GasTipCap)

	auth.Value = big.NewInt(0)
	auth.Context = ctx
	auth.Value = nil
	auth.GasLimit = 300000

	return auth, nil
}
