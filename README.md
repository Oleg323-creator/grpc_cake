# QuoteSwap gRPC Service (grpc_cake)

QuoteSwap is a gRPC API that allows you to get quotes and execute token swaps on PancakeSwap V2 and V3 across multiple chains: Ethereum, BSC, and Base.

## How to Run

1. Clone the repo:
   ```git clone https://github.com/github_username/repo_name.git```

2. Configure environment variables in a `.env` file or export manually:
## Environment Variables

| Variable         | Description                                                       |
|------------------|-------------------------------------------------------------------|
| `API_KEY`        | (Optional) API key for RPC provider if needed                     |
| `CHAIN_BSC`      | RPC endpoint for Binance Smart Chain                              |
| `CHAIN_ETH`      | RPC endpoint for Ethereum network                                 |
| `CHAIN_BASE`     | RPC endpoint for Base network                                     |
| `PRIVATE_KEY`    | Private key for signing transactions                              |
| `RECIPIENT_ADDR` | Address to receive tokens after swap                              |
| `GRPC_PORT`      | gRPC server port (default: 50051)                                 |
 
   ```
   
   # RPC URLs (Infura, Alchemy, or your own)
   CHAIN_BSC=https://bsc-mainnet.infura.io/v3/
   CHAIN_ETH=https://mainnet.infura.io/v3/
   CHAIN_BASE=https://base-mainnet.infura.io/v3/

   ```

3. Run the server:
   ```bash
   go run main.go
   ```

## Example Requests

### GetQuote
```bash
grpcurl -plaintext -d '{
  "token_in": "0x4200000000000000000000000000000000000006",
  "token_out": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
  "amount": "1000000000",
  "dex": "v3",
  "slippage_bps": 30,
  "chain": "base"
}' localhost:50051 quoteswap.QuoteSwapService/GetQuote

###Response:
{
  "inputToken": "0x4200000000000000000000000000000000000006",
  "inAmount": "1000000000",
  "outputToken": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",
  "outAmount": "1",
  "slippageBps": 30,
  "dex": "v3",
  "chain": "base"
}

```

### ExecuteSwap
```bash
grpcurl -plaintext -d '{
  "quoting_response": {
     "inputToken": "0x4200000000000000000000000000000000000006",
  "inAmount": "1000000000",
  "outputToken": "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913",  
  "outAmount": "1",
  "slippageBps": 30,
  "dex": "v3",
  "chain": "base"

  }
}' localhost:50051 quoteswap.QuoteSwapService/ExecuteSwap

###Response:
{
  "transactionHash": "0x7c3ffabf6488b52cf480ba121753599174de637ce4b7c84459001ce3e9c5ac1e",
  "status": "PENDING",
  "sellTokenQty": 1e+09,
  "executedPrice": 1
}

```

> NOTE: You must set `RECIPIENT_ADDR` and `PRIVATE_KEY` in the environment for swap execution.

##  Architecture Overview

- **main.go** initializes the gRPC server and sets up V2 and V3 services per chain.
- **Service Routing**:
    - The request’s `chain` and `dex` fields determine which implementation to use.
    - Internally routes to either V2 or V3 logic using a shared `Swapper` interface.
- **QuoteSwapServiceServer** is the main handler for:
    - `GetQuote` — estimates output amount.
    - `ExecuteSwap` — signs and sends a swap transaction.
- **Blockchain client** encapsulates JSON-RPC interactions per chain, and ABI gens.

## Limitations

- Only supports **PancakeSwap** (V2 and V3).
- Requires reliable RPC endpoints for target chains.
- The `outAmount` in `ExecuteSwap` must come from a prior `GetQuote` call.
- Swap execution assumes sufficient token approvals and balances on sender.