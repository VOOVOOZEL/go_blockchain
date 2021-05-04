package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

const (
	difficulty = 1

	genesisCoinbaseData = "The Times 03/Jan/2009 Chancellor on brink of second bailout for banks"
)

// Block represents each 'item' in the blockchain
type Block struct {
	Timestamp    string
	Transactions []*Transaction
	Hash         string
	PrevHash     string
	Nonce        string
}

// Blockchain is a series of validated Blocks
type Blockchain struct {
	sync.Mutex
	blocks []*Block
}

func NewGenesisBlock() *Block {
	genesisBlock := &Block{}
	return &Block{time.Now().String(), []*Transaction{NewCoinbaseTX("Ivan", genesisCoinbaseData)}, calculateHash(genesisBlock), "", ""}
}

func (bc *Blockchain) AddBlock(newBlock *Block) {
	bc.Lock()
	defer bc.Unlock()
	bc.blocks = append(bc.blocks, newBlock)
}

func NewBlockchain() Blockchain {
	genesisBlock := NewGenesisBlock()
	spew.Dump(genesisBlock)
	return Blockchain{sync.Mutex{}, []*Block{genesisBlock}}
}

// SendMessage takes incoming JSON payload for writing heart rate
type SendMessage struct {
	From, To string
	Value    int
}

// SendMessage takes incoming JSON payload for writing heart rate
type BalanceMessage struct {
	Address string
}

var (
	bc Blockchain
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	bc = NewBlockchain()
	log.Fatal(run())
}

// web server
func run() error {
	mux := makeMuxRouter()
	httpPort := os.Getenv("PORT")
	log.Println("HTTP Server Listening on port :", httpPort)
	s := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}

	if err := s.ListenAndServe(); err != nil {
		return err
	}

	return nil
}

// create handlers
func makeMuxRouter() http.Handler {
	muxRouter := mux.NewRouter()
	muxRouter.HandleFunc("/", handleGetBlockchain).Methods("GET")
	muxRouter.HandleFunc("/", handleWriteBlock).Methods("POST")
	muxRouter.HandleFunc("/balance", handleGetBalance).Methods("POST")
	return muxRouter
}

// write blockchain when we receive an http request
func handleGetBlockchain(w http.ResponseWriter, r *http.Request) {
	bytes, err := json.MarshalIndent(bc.blocks, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, string(bytes))
}

// takes JSON payload as an input for heart rate (BPM)
func handleWriteBlock(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var m SendMessage

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&m); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	newBlock, err := generateBlock(bc.blocks[len(bc.blocks)-1], m)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	if isBlockValid(newBlock, bc.blocks[len(bc.blocks)-1]) {
		bc.AddBlock(newBlock)
		spew.Dump(bc.blocks)
	}

	respondWithJSON(w, r, http.StatusCreated, newBlock)

}

// takes JSON payload as an input for heart rate (BPM)
func handleGetBalance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var m BalanceMessage

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&m); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	balance := 0
	UTXOs := bc.FindUTXO(m.Address)

	for _, out := range UTXOs {
		balance += out.Value
	}

	respondWithJSON(w, r, http.StatusCreated, balance)

}

func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("HTTP 500: Internal Server Error"))
		return
	}
	w.WriteHeader(code)
	w.Write(response)
}

// make sure block is valid by checking index, and comparing the hash of the previous block
func isBlockValid(newBlock, oldBlock *Block) bool {
	if oldBlock.Hash != newBlock.PrevHash {
		return false
	}

	if calculateHash(newBlock) != newBlock.Hash {
		return false
	}

	return true
}

// SHA256 hasing
func calculateHash(block *Block) string {
	record := block.Timestamp + block.PrevHash + block.Nonce
	h := sha256.New()
	h.Write(append([]byte(record), block.HashTransactions()...))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

// create a new block using previous block's hash
func generateBlock(oldBlock *Block, m SendMessage) (*Block, error) {
	newBlock := new(Block)

	t := time.Now()
	newTranaction, err := NewUTXOTransaction(m.From, m.To, m.Value, &bc)
	if err != nil {
		return nil, err
	}
	newBlock.Timestamp = t.String()
	newBlock.Transactions = []*Transaction{newTranaction}
	newBlock.PrevHash = oldBlock.Hash

	for i := 0; ; i++ {
		newBlock.Nonce = fmt.Sprintf("%x", i)
		newHash := calculateHash(newBlock)
		if !isHashValid(newHash, difficulty) {
			fmt.Println(newHash, " do more work!")
			continue
		}
		fmt.Println(newHash, " work done!")
		newBlock.Hash = newHash
		break

	}
	return newBlock, nil
}

func isHashValid(hash string, difficulty int) bool {
	prefix := strings.Repeat("0", difficulty)
	return strings.HasPrefix(hash, prefix)
}

func (tx *Transaction) name() {

}

// FindUnspentTransactions returns a list of transactions containing unspent outputs
func (bc *Blockchain) FindUnspentTransactions(address string) []*Transaction {
	var unspentTXs []*Transaction
	spentTXOs := make(map[string][]int)

	for i := len(bc.blocks) - 1; i >= 0; i-- {
		if len(bc.blocks[i].Transactions) == 0 {
			return unspentTXs
		}

		for _, tx := range bc.blocks[i].Transactions {
			if !tx.IsCoinbase() {
				for _, in := range tx.Vin {
					if in.CanUnlockOutputWith(address) {
						spentTXOs[in.Txid] = append(spentTXOs[in.Txid], in.Vout)
					}
				}
			}
		Outputs:
			for outIdx, out := range tx.Vout {
				for _, spentOut := range spentTXOs[tx.ID] {
					if spentOut == outIdx {
						continue Outputs
					}
				}

				if out.CanBeUnlockedWith(address) {
					unspentTXs = append(unspentTXs, tx)
				}
			}
		}
	}

	return unspentTXs
}

// FindUTXO finds and returns all unspent transaction outputs
func (bc *Blockchain) FindUTXO(address string) []TXOutput {
	var UTXOs []TXOutput
	unspentTransactions := bc.FindUnspentTransactions(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Vout {
			if out.CanBeUnlockedWith(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}

	return UTXOs
}

// FindSpendableOutputs finds and returns unspent outputs to reference in inputs
func (bc *Blockchain) FindSpendableOutputs(address string, amount int) (
	int, map[string][]int) {

	unspentOutputs := make(map[string][]int)
	unspentTXs := bc.FindUnspentTransactions(address)
	accumulated := 0

	for _, tx := range unspentTXs {
		for idx, out := range tx.Vout {
			if out.CanBeUnlockedWith(address) && accumulated < amount {
				accumulated += out.Value
				unspentOutputs[tx.ID] = append(unspentOutputs[tx.ID], idx)

				if accumulated >= amount {
					return accumulated, unspentOutputs
				}
			}
		}
	}
	return accumulated, unspentOutputs
}

func (b *Block) HashTransactions() []byte {
	var txHashes [][]byte
	var txHash [32]byte

	for _, tx := range b.Transactions {
		txHashes = append(txHashes, []byte(tx.ID))
	}
	txHash = sha256.Sum256(bytes.Join(txHashes, []byte{}))

	return txHash[:]
}
