package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	RPC_PORT                = 9332
	RPC_USERNAME            = "user"
	RPC_PASSWORD            = "pass"
	RPC_HOST                = "127.0.0.1"
	BLOCK_RETARGET_INTERVAL = 2016
	BLOCK_BIP9_INTERVAL     = 2016 * 4
)

func BuildURL() string {
	return fmt.Sprintf("http://%s:%s@%s:%d", RPC_USERNAME, RPC_PASSWORD, RPC_HOST, RPC_PORT)
}

func SendHTTPGetRequest(url string, jsonDecode bool, result interface{}) (err error) {
	res, err := http.Get(url)

	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		log.Printf("HTTP status code: %d\n", res.StatusCode)
		return errors.New("Status code was not 200.")
	}

	contents, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return err
	}

	defer res.Body.Close()

	if jsonDecode {
		err := JSONDecode(contents, &result)

		if err != nil {
			return err
		}
	} else {
		result = &contents
	}

	return nil
}

func JSONDecode(data []byte, to interface{}) error {
	err := json.Unmarshal(data, &to)

	if err != nil {
		return err
	}

	return nil
}

func EncodeURLValues(url string, values url.Values) string {
	path := url
	if len(values) > 0 {
		path += "?" + values.Encode()
	}
	return path
}

func SendRPCRequest(method, req interface{}) (map[string]interface{}, error) {
	var params []interface{}
	if req != nil {
		params = append(params, req)
	} else {
		params = nil
	}

	data, err := json.Marshal(map[string]interface{}{
		"method": method,
		"id":     1,
		"params": params,
	})

	if err != nil {
		return nil, err
	}

	resp, err := http.Post(BuildURL(), "application/json", strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	result := make(map[string]interface{})
	err = json.Unmarshal(body, &result)
	if err != nil {
		return nil, err
	}

	if result["error"] != nil {
		errorMsg := result["error"].(map[string]interface{})
		return nil, fmt.Errorf("Error code: %v, message: %v\n", errorMsg["code"], errorMsg["message"])
	}
	return result, nil
}

func GetBlockHeight() (int, error) {
	result, err := SendRPCRequest("getinfo", nil)
	if err != nil {
		return 0, err
	}

	result = result["result"].(map[string]interface{})
	block := result["blocks"].(float64)
	return int(block), nil
}

func GetBlockHash(block int) (string, error) {
	result, err := SendRPCRequest("getblockhash", block)
	if err != nil {
		return "", err
	}
	return result["result"].(string), nil
}

func GetBlockCollated(block int) (int, error) {
	result, err := SendRPCRequest("getblockhash", block)
	if err != nil {
		return 0, err
	}

	blockHash := result["result"].(string)

	result, err = SendRPCRequest("getblock", blockHash)
	if err != nil {
		return 0, err
	}
	result = result["result"].(map[string]interface{})
	version := result["version"].(float64)
	log.Printf("Block %s height %d has version %s\n", blockHash, block, fmt.Sprintf("0x%08x", int(version)))
	return int(version), nil
}

func PrintBlockSummary(blockIndex map[int]int, blockHeight, interval int) {
	log.Printf("Current block height: %d\n", blockHeight)
	log.Printf("Block range: %d to %d\n", blockHeight-interval+1, blockHeight)
	totalBlocks := 0
	for version, count := range blockIndex {
		totalBlocks += count
		var percentage float64 = (float64(count) / float64(interval)) * 100 / 1
		log.Printf("%d version %s blocks (%.2f%%).", count, fmt.Sprintf("0x%08x", version), percentage)
	}
	log.Printf("Total blocks: %d\n", totalBlocks)
}

func GetNextBlockRetarget(blockNum int) int {
	iterations := 0
	for i := 0; i < blockNum; i += BLOCK_RETARGET_INTERVAL {
		iterations++
	}
	if iterations*BLOCK_RETARGET_INTERVAL == blockNum {
		iterations++
	}
	return iterations * BLOCK_RETARGET_INTERVAL
}

func main() {
	currentHeight, err := GetBlockHeight()
	if err != nil {
		log.Fatal(err)
	}

	blockIndex := map[int]int{}
	interval := BLOCK_BIP9_INTERVAL
	log.Printf("Current block height: %d\n", currentHeight)
	log.Printf("Next block retarget %d\n", GetNextBlockRetarget(currentHeight))
	blockStart := currentHeight - interval + 1

	for ; blockStart <= currentHeight; blockStart++ {
		log.Println(blockStart)
		blockVersion, err := GetBlockCollated(blockStart)
		if err != nil {
			log.Println(err)
		}
		blockIndex[blockVersion] += 1
	}

	PrintBlockSummary(blockIndex, currentHeight, interval)

	for {
		newHeight, err := GetBlockHeight()
		if err != nil {
			log.Fatal(err)
		}

		if newHeight != currentHeight {
			diff := newHeight - currentHeight
			log.Printf("New height: %d Old height: %d Diff: %d\n", newHeight, currentHeight, diff)

			for i := newHeight; i > currentHeight; i-- {
				log.Printf("Adding %d.\n", i)
				blockVer, err := GetBlockCollated(i)
				if err != nil {
					log.Fatal(err)
				}

				blockIndex[blockVer] += 1
			}

			for i := newHeight - interval; i < (newHeight-interval)+diff; i++ {
				log.Printf("Removing %d.\n", i)
				blockVer, err := GetBlockCollated(i)
				if err != nil {
					log.Fatal(err)
				}
				blockIndex[blockVer] -= 1
			}
			currentHeight = newHeight
			PrintBlockSummary(blockIndex, currentHeight, interval)
		} else {
			time.Sleep(time.Second)
		}
	}
}
