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
		fmt.Printf("%s: \033[31m%s %s\033[0m\n", url, res.Status, res.Body)
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

	for {
		//chickenPlan.Execute(characterName, token)
		woodPlan.Execute("Christopf", token)
	}
}

type Action interface {
	Do(character, token string)
}

type URLAction struct {
	URL  string
	Data map[string]any
}

func (u URLAction) Do(character, token string) {
	do(fmt.Sprintf(u.URL, character), token, u.Data)
}

type GenericAction struct {
	fn func(character, token string)
}

func (g GenericAction) Do(character, token string) {
	g.fn(character, token)
}

var (
	RestAction          = URLAction{URL: "my/%s/action/rest"}
	FightAction         = URLAction{URL: "my/%s/action/fight"}
	GatherAction        = URLAction{URL: "my/%s/action/gathering"}
	ShowInventoryAction = GenericAction{fn: func(character, token string) {
		charInfo := must(do("characters/"+character, token, nil))
		fmt.Println("inventory:")
		for _, el := range charInfo["inventory"].([]any) {
			items := el.(map[string]any)
			if items["code"].(string) == "" {
				continue
			}

			fmt.Printf("\t\033[33m%-4.0f\033[0m %s\n", items["quantity"].(float64), items["code"].(string))
		}
	}}
)

func MoveAction(x, y int) Action {
	return URLAction{URL: "my/%s/action/move", Data: map[string]any{"x": x, "y": y}}
}

func MovePOIAction(p POI) Action {
	return URLAction{URL: "my/%s/action/move", Data: map[string]any{"x": p.X, "y": p.Y}}
}

type Condition func(charInfo map[string]any) bool

func PositionCondition(x, y int) Condition {
	return func(charInfo map[string]any) bool {
		return charInfo["x"].(float64) == float64(x) && charInfo["y"].(float64) == float64(y)
	}
}

func POICondition(p POI) Condition {
	return func(charInfo map[string]any) bool {
		return charInfo["x"].(float64) == p.X && charInfo["y"].(float64) == p.Y
	}
}

func NotCondition(c Condition) Condition {
	return func(charInfo map[string]any) bool {
		return !c(charInfo)
	}
}

func FullInventoryCondition() Condition {
	return func(charInfo map[string]any) bool {
		var items float64
		for _, slot := range charInfo["inventory"].([]any) {
			items += slot.(map[string]any)["quantity"].(float64)
		}

		return items >= charInfo["inventory_max_items"].(float64)
	}
}

func ReadyCondition() Condition {
	return func(charInfo map[string]any) bool {
		expiration := must(time.Parse(time.RFC3339, charInfo["cooldown_expiration"].(string)))
		if time.Since(expiration).Seconds() < 0 {
			fmt.Printf("downtime to \033[36m%s\033[0m\n", charInfo["cooldown_expiration"].(string))
			return false
		}
		return true
	}
}

func AlwaysCondition(_ map[string]any) bool { return true }

type Alternative struct {
	name      string
	actions   []Action
	condition Condition
}

type POI struct {
	X, Y float64
}

var (
	ChickenPOI = POI{X: 0, Y: 1}
	WoodPOI    = POI{X: -1, Y: 0}
)

var (
	chickenPlan = Plan{
		name:          "farm chicken",
		preConditions: []Condition{NotCondition(FullInventoryCondition())},
		alternatives: []Alternative{
			{
				name: "rest if almost dead",
				condition: func(charInfo map[string]any) bool {
					return charInfo["hp"].(float64) < 5
				},
				actions: []Action{RestAction},
			},
			{
				name:      "move to chicken place",
				condition: NotCondition(POICondition(ChickenPOI)),
				actions:   []Action{MovePOIAction(ChickenPOI)},
			},
			{
				name:      "fight chicken",
				condition: AlwaysCondition,
				actions:   []Action{FightAction, ShowInventoryAction},
			},
		},
	}

	woodPlan = Plan{
		name:          "cut wood",
		preConditions: []Condition{ReadyCondition(), NotCondition(FullInventoryCondition())},
		alternatives: []Alternative{
			{
				name:      "move to forest",
				condition: NotCondition(POICondition(WoodPOI)),
				actions:   []Action{MovePOIAction(WoodPOI)},
			},
			{
				name:      "cut some wood",
				condition: AlwaysCondition,
				actions:   []Action{GatherAction, ShowInventoryAction},
			},
		},
	}
)

type Plan struct {
	name          string
	preConditions []Condition
	alternatives  []Alternative
}

func (p *Plan) Execute(character, token string) {
	charInfo := must(do("characters/"+character, token, nil))
	for _, cond := range p.preConditions {
		if !cond(charInfo) {
			fmt.Println("can't execute plan: precondition not met")
			return
		}
	}

	// always check and enforce cooldown before trying any task
	expiration := must(time.Parse(time.RFC3339, charInfo["cooldown_expiration"].(string)))
	if time.Since(expiration).Seconds() < 0 {
		fmt.Printf("downtime to \033[36m%s\033[0m\n", charInfo["cooldown_expiration"].(string))
		time.Sleep(time.Until(expiration))
	}

	for _, alt := range p.alternatives {
		if alt.condition(charInfo) {
			for _, action := range alt.actions {
				action.Do(character, token)
			}
			return
		}
	}
}
