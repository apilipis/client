// Connect to Ethereum node and make requests
package rpc

import (
  "encoding/json"
  "io/ioutil"
  "net/http"
  "log"
  "strconv"
  "math/big"
)
import "fmt"
import "sig"
import "github.com/ethereum/go-ethereum/crypto"


// Global client connection
var client = EthereumClient{}

const DEFAULT_GAS = 100000
const DEFAULT_GAS_PRICE = 2000000000

/**
 * Make initial connection to RPC provider. Save that connection in memory.
 *
 * @param provider    Full URI of the RPC provider, including the protocol
 *                    and port
 */
func ConnectToRPC(provider string) {
  client = EthereumClient{provider}
  log.Print("Connecting to Ethereum provider ", provider)
  block, err := client.Eth_blockNumber()
  if err != nil {
    log.Fatal("Could not reach Ethereum provider.")
  } else {
    log.Print("Connected to RPC. Starting block=", block)
  }
}


/**
 * Check if the serial number has been registered with the registry contract.
 *
 * @param from             The origin of the message
 * @param hashed_serial    The original setup address for the agent
 * @param registry         Address of the registry contract
 * @return                 true if registerd, false if not
 */
func CheckRegistered(from string, hashed_serial string, registry string) (bool) {
  // registered(bytes32) --> 5524d548
  call := Call{From: from, To: registry, Data: "0x5524d548"+hashed_serial }
  registered, err := client.Eth_call(call)
  if err != nil {
    log.Fatal("Could not check if agent was registered: ", err)
  }
  pass, _ := strconv.ParseUint(registered, 0, 64)
  if pass == 0 { return false }
  return true
}


/**
 * Check if a specific address is registered to a specific serial number
 *
 * @param from             The origin of the message
 * @param hashed_serial    The original setup address for the agent
 * @param address          The address to check against the serial number
 * @param registry         Address of the registry contract
 * @return                 true if registerd, false if not
 */
func CheckRegistry(from string, hashed_serial string, address string, registry string) (bool) {
  // check_registry(bytes32,address) --> fc91446d
  call := Call{From: from, To: registry, Data: "0xfc91446d"+hashed_serial+Zfill(address)}
  registered, err := client.Eth_call(call)
  if err != nil {
    log.Fatal("Could not check if agent was registered: ", err)
  }
  pass, _ := strconv.ParseUint(registered, 0, 64)
  if pass == 0 { return false }
  return true
}


/**
 * Check if the wallet address has been claimed by a human owner.
 *
 * @param serial_hash    Hash of the agent's serial number
 * @param registry       Address of the registry contract
 * @return               true if registerd, false if not
 */
func CheckClaimed(serial_hash string, registry string) (bool) {
  // claimed(bytes32) --> c884ef83
  call := Call{From: serial_hash, To: registry, Data: "0xcc3c0f06"+serial_hash}
  claimed, err := client.Eth_call(call)
  if err != nil {
    log.Fatal("Could not check if agent was claimed: ", err)
  }
  pass, _ := strconv.ParseUint(claimed, 0, 64)
  if pass == 0 { return false }
  return true
}


/**
 * Add a wallet address to the registry. This will bump the existing setup
 * address from the registry and all future requests should be made with
 * respect to the wallet.
 *
 * @param from    Setup address
 * @param to      Registry contract adress
 * @param data    Hex string with data payload
 * @param API     Full base URI of the hub API
 * @param pkey    Private key of the currently registered setup keypair
 * @return        error, txhash
 */
func AddWallet(from string, to string, data string, API string, pkey string) (error, string, int64) {
  // Convert pkey to an ecdsa privat ekey object
  privkey, err := crypto.HexToECDSA(pkey)
  if err != nil {
    return fmt.Errorf("Could not parse private key: (%s)", err), "", 0
  }
  // Get some params
  gas, gasPrice := DefaultGas(API)
  _nonce := GetNonce(from)
  nonce, err2 := strconv.ParseUint(_nonce[2:], 16, 64)
  if err2 != nil {
    return fmt.Errorf("Could not parse nonce: (%s)", err2), "", 0
  }
  net_version, err3 := client.NetVersion()
  if err3 != nil {
    return fmt.Errorf("Could not get network version: (%s)", err3), "", 0
  }
  // Form the raw transaction (signed payload)
  txn, err4 := sig.GetRawTx(net_version, from, to, data, nonce, 0, gas, gasPrice, privkey)
  if err4 != nil {
    return fmt.Errorf("Error signing tx: (%s)", err4), "", 0
  }
  // Submit the raw transaction to our RPC client
  txhash, err4 := client.Eth_sendRawTransaction(txn)
  if err4 != nil {
    return fmt.Errorf("Error submitting tx: (%s)", err4), "", 0
  }
  // Return the txhash
  return nil, txhash, gas.Int64()
}


/**
 * Check a receipt for the cumulative gas used. This will be our metric to check
 * if the tx threw.
 *
 * @param txhash    Amount of gas used, an integer
 * @return          (cumulativeGasUsed: 0x-prefixed hex string, error)
 */
func CheckReceipt(txhash string) (int64, error) {
  _gasUsed, err := client.Eth_gasUsed(txhash)
  if err != nil {
    return 0, fmt.Errorf("Error getting tx receipt: (%s)", err)
  } else if _gasUsed == "" {
    return 0, nil
  }
  gasUsed, _ := strconv.ParseInt(_gasUsed, 0, 64)
  return gasUsed, nil
}


/**
 * Get the nonce (transaction count) of the address.
 *
 * @param addr    Address to be checked
 * @return        Hex string representation of the nonce
 */
func GetNonce(addr string) (string) {
  nonce, err := client.Eth_getTransactionCount(addr)
  if err != nil {
    log.Panic("Could not reach Ethereum provider.")
  }
  return nonce
}

// =======================
// UTILITY functions
// =======================

type GasRes struct {
  Gas int `json:"gas"`
  GasPrice int `json:"gasPrice"`
}

/**
 * Query the API for the default gas and gas prices. Return hardcoded
 * default values if the API errors out.
 *
 * @param  api    Full base URI of the api
 * @return        (gas, gasprice in wei) - both are big.Int pointers
 */
func DefaultGas(api string) (*big.Int, *big.Int) {
  var gas = big.NewInt(int64(DEFAULT_GAS))
  var gasPrice = big.NewInt(int64(DEFAULT_GAS_PRICE))
  var result = new(GasRes)
  res, err := http.Get(api+"/Gas")
  if err == nil {
    body, err2 := ioutil.ReadAll(res.Body)
    if err2 == nil {
      err3 := json.Unmarshal(body, &result)
      if err3 == nil {
        gas = big.NewInt(int64(result.Gas))
        gasPrice = big.NewInt(int64(result.GasPrice))
      }
    }
  }
  return gas, gasPrice
}

// Remove the 0x prefix if it exists
func unprefix(s string) (string) {
  if s[:2] == "0x" { return s[2:] }
  return s
}

// Left pad a string up to 64 characters with 0s
func Zfill(s string) (string) {
  // Cut off any rouge 0x prefixes
  if (s[:2] == "0x") { s = s[2:]}
  var pad = ""
  for i := 0; i < (64-len(s)); i++ {
		pad += "0"
	}
  return pad + s
}