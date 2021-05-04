package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

const difficulty = 6

// Block represents each 'item' in the blockchain
type Block struct {
	Timestamp string
	Value     int
	Hash      string
	PrevHash  string
	Nonce     string
}

// Blockchain is a series of validated Blocks
type Blockchain struct {
	sync.Mutex
	blocks []*Block
}

func NewGenesisBlock() *Block {
	genesisBlock := &Block{}
	return &Block{time.Now().String(), 0, calculateHash(genesisBlock), "", ""}
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

// Message takes incoming JSON payload for writing heart rate
type Message struct {
	Value int
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
	var m Message

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&m); err != nil {
		respondWithJSON(w, r, http.StatusBadRequest, r.Body)
		return
	}
	defer r.Body.Close()

	newBlock := generateBlock(bc.blocks[len(bc.blocks)-1], m.Value)

	if isBlockValid(newBlock, bc.blocks[len(bc.blocks)-1]) {
		bc.AddBlock(newBlock)
		spew.Dump(bc.blocks)
	}

	respondWithJSON(w, r, http.StatusCreated, newBlock)

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
	record := block.Timestamp + strconv.Itoa(block.Value) + block.PrevHash + block.Nonce
	h := sha256.New()
	h.Write([]byte(record))
	hashed := h.Sum(nil)
	return hex.EncodeToString(hashed)
}

// create a new block using previous block's hash
func generateBlock(oldBlock *Block, value int) *Block {
	newBlock := new(Block)

	t := time.Now()

	newBlock.Timestamp = t.String()
	newBlock.Value = value
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
	return newBlock
}

func isHashValid(hash string, difficulty int) bool {
	prefix := strings.Repeat("0", difficulty)
	return strings.HasPrefix(hash, prefix)
}
