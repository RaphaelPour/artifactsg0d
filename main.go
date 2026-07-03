package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	baseURL       = "https://api.artifactsmmo.com/"
	characterName = "Holzfred"
)

func must[T any](val T, err error) T {
	if err != nil {
		panic(err)
	}
	return val
}

func do(url, token string, body map[string]any) (map[string]any, error) {

	var req *http.Request
	var err error
	effUrl := baseURL + url
	if body != nil {
		dumpedBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}

		req, err = http.NewRequest("POST", effUrl, bytes.NewReader(dumpedBody))
		if err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", "application/json")
	} else if strings.Contains(url, "action") {
		req, err = http.NewRequest("POST", effUrl, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest("GET", effUrl, nil)
		if err != nil {
			return nil, err
		}
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	res, err := http.DefaultClient.Do(req)
	defer func() {
		if res != nil {
			res.Body.Close()
		}
	}()
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		fmt.Printf("%s: \033[31m%s\033[0m\n", url, res.Status)
		os.Exit(1)
	}

	fmt.Printf("%s: \033[32m%s\033[0m\n", url, res.Status)

	if res.Body == nil {
		return nil, nil
	}
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if len(responseBody) > 0 {
		parsedResponseBody := make(map[string]any)
		if err := json.Unmarshal(responseBody, &parsedResponseBody); err != nil {
			return nil, err
		}

		return parsedResponseBody["data"].(map[string]any), nil
	}
	return nil, nil
}

func main() {
	token, ok := os.LookupEnv("ARTIFACTS_TOKEN")
	if !ok {
		fmt.Println("ARTIFACTS_TOKEN env is missing")
		return
	}

	// check if char is existing
	_ = must(do("characters/Holzfred", token, nil))

	for {
		// get health
		character := must(do("characters/Holzfred", token, nil))
		expiration := must(time.Parse(time.RFC3339, character["cooldown_expiration"].(string)))
		if time.Since(expiration).Seconds() < 0 {
			fmt.Printf("downtime to \033[36m%s\033[0m\n", character["cooldown_expiration"].(string))
			time.Sleep(time.Until(expiration))
			continue
		}

		// health < 10 then reset
		if character["hp"].(float64) < 10 {
			must(do("my/Holzfred/action/rest", token, nil))
			continue
		}

		// position != chicken farm? then move
		if character["x"].(float64) != 0 && character["y"].(float64) != -1 {
			must(do("my/Holzfred/action/move", token, map[string]any{"x": 0, "y": -1}))
			continue
		}

		// otherwise fight
		must(do("my/Holzfred/action/fight", token, nil))
	}
}
