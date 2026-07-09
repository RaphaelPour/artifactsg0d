package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	baseURL = "https://api.artifactsmmo.com/"
)

var (
	characters = []string{
		"Holzfred", "Christopf", "Waffelisa", "Farmina",
	}
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

	if res.StatusCode == http.StatusTooManyRequests {
		fmt.Printf("[%-10s] \033[33mRate-Limit\033[0m\n", url)
		time.Sleep(5 * time.Second)
		return do(url, token, body)
	}

	if res.StatusCode == 490 {
		return nil, nil
	}

	if res.StatusCode != http.StatusOK {
		fmt.Printf("[%-10s] \033[31m%s %s\033[0m\n", url, res.Status, res.Body)
		os.Exit(1)
	}

	fmt.Printf("[%-10s] \033[32m%s\033[0m\n", url, res.Status)

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

func discoverToken() (string, error) {
	token, ok := os.LookupEnv("ARTIFACTS_TOKEN")
	if ok {
		return token, nil
	}

	content, err := os.ReadFile(".token")
	if err != nil {
		return "", fmt.Errorf("ARTIFACTS_TOKEN or .token file must be provided")
	}

	return strings.TrimSpace(string(content)), nil
}

func characterLoop(ctx context.Context, character, token string) {
	for ctx.Err() == nil {
		charInfo := must(do("characters/"+character, token, nil))

		// always check and enforce cooldown before trying any task
		expiration := must(time.Parse(time.RFC3339, charInfo["cooldown_expiration"].(string)))
		if time.Since(expiration).Seconds() < 0 {
			fmt.Printf("[%-10s] downtime to \033[36m%s\033[0m\n", character, charInfo["cooldown_expiration"].(string))
			time.Sleep(time.Until(expiration))
		}

		var proofOfWork bool
		for _, plan := range plans {
			if plan.IsExecutable(character, token) {
				plan.Execute(character, token)
				proofOfWork = true
				break
			}
		}

		if !proofOfWork {
			fmt.Printf("[%-10s] nothing to do. Napping a minute...\n", character)
			time.Sleep(time.Minute)
		}
		//chickenPlan.Execute(characterName, token)
		//woodPlan.Execute(characters[0], token)
	}
}

func main() {
	token, err := discoverToken()
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx := context.Background()
	for _, character := range characters {
		go characterLoop(ctx, character, token)
	}

	<-ctx.Done()
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

type ChooseAction struct {
	actions []Action
}

func (c ChooseAction) Do(character, token string) {
	c.actions[rand.IntN(len(c.actions))].Do(character, token)
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

			fmt.Printf("[%-10s] \t\033[33m%-4.0f\033[0m %s\n", character, items["quantity"].(float64), items["code"].(string))
		}
	}}
	ShowHealthAction = GenericAction{fn: func(character, token string) {
		charInfo := must(do("characters/"+character, token, nil))
		fmt.Printf("[%-10s] health: %.0f\n", character, charInfo["hp"].(float64))
	}}
)

func ChooseActionHelper(actions ...Action) Action {
	return ChooseAction{actions: actions}
}

func MoveAction(x, y int) Action {
	return URLAction{URL: "my/%s/action/move", Data: map[string]any{"x": x, "y": y}}
}

func MovePOIAction(p POI) Action {
	return URLAction{URL: "my/%s/action/move", Data: map[string]any{"x": p.X, "y": p.Y}}
}

func CraftAction(item string, quantity int) Action {
	return URLAction{URL: "my/%s/action/crafting", Data: map[string]any{"code": item, "quantity": quantity}}
}

func UseItemAction(item string, qty int) Action {
	return URLAction{URL: "my/%s/action/use", Data: map[string]any{"code": item, "quantity": qty}}
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

func LowHealthCondition(hp int) Condition {
	return func(charInfo map[string]any) bool {
		return charInfo["hp"].(float64) < float64(hp)
	}
}

func MinLevelCondition(lvl int) Condition {
	return func(charInfo map[string]any) bool {
		return charInfo["level"].(float64) >= float64(lvl)
	}
}
func MinHealthCondition(hp int) Condition {
	return func(charInfo map[string]any) bool {
		return charInfo["hp"].(float64) >= float64(hp)
	}
}

func ChanceCondition(chance float64) Condition {
	return func(_ map[string]any) bool {
		return rand.Float64() <= chance
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

func HasItemCondition(itemName string, qty int) Condition {
	return func(charInfo map[string]any) bool {
		for _, slot := range charInfo["inventory"].([]any) {
			item := slot.(map[string]any)
			if item["code"] == itemName {
				return item["quantity"].(float64) >= float64(qty)
			}
		}
		return false
	}
}

func ReadyCondition() Condition {
	return func(charInfo map[string]any) bool {
		expiration := must(time.Parse(time.RFC3339, charInfo["cooldown_expiration"].(string)))
		if time.Since(expiration).Seconds() < 0 {
			fmt.Printf("[%-10s] downtime to \033[36m%s\033[0m\n", charInfo["name"], charInfo["cooldown_expiration"].(string))
			return false
		}
		return true
	}
}

func AndCondition(c ...Condition) Condition {
	return func(charInfo map[string]any) bool {
		for _, cond := range c {
			if !cond(charInfo) {
				return false
			}
		}
		return true
	}
}

func OrCondition(c ...Condition) Condition {
	return func(charInfo map[string]any) bool {
		for _, cond := range c {
			if cond(charInfo) {
				return true
			}
		}
		return true
	}
}

func XorCondition(a, b Condition) Condition {
	return func(charInfo map[string]any) bool {
		return a(charInfo) != b(charInfo)
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
	ChickenPOI    = POI{X: 0, Y: 1}
	CowPOI        = POI{X: 0, Y: 2}
	CookingPOI    = POI{X: 1, Y: 1}
	WoodPOI       = POI{X: -1, Y: 0}
	BankPOI       = POI{X: 4, Y: 1}
	GreenSlimePOI = POI{X: 0, Y: -1}
	RedSlimePOI   = POI{X: 1, Y: -1}
	BlueSlimePOI  = POI{X: 2, Y: -1}
)

var (
	chickenPlan = Plan{
		name:          "farm chicken",
		preConditions: []Condition{AndCondition(ChanceCondition(0.5), NotCondition(FullInventoryCondition()))},
		alternatives: []Alternative{
			{
				name:      "eat food if almost dead",
				condition: AndCondition(LowHealthCondition(100), HasItemCondition("cooked_chicken", 1)),
				actions:   []Action{UseItemAction("cooked_chicken", 1), ShowHealthAction},
			},
			{
				name:      "rest if almost dead",
				condition: LowHealthCondition(50),
				actions:   []Action{RestAction, ShowHealthAction},
			},
			{
				name:      "move to chicken place",
				condition: NotCondition(POICondition(ChickenPOI)),
				actions:   []Action{MovePOIAction(ChickenPOI)},
			},
			{
				name:      "fight chicken",
				condition: AlwaysCondition,
				actions:   []Action{FightAction, ShowInventoryAction, ShowHealthAction},
			},
		},
	}

	cowPlan = Plan{
		name: "farm cow",
		preConditions: []Condition{
			AndCondition(
				MinHealthCondition(100),
				MinLevelCondition(10),
				ChanceCondition(0.5),
				NotCondition(FullInventoryCondition()),
			),
		},
		alternatives: []Alternative{
			{
				name:      "eat food if almost dead",
				condition: AndCondition(LowHealthCondition(100), HasItemCondition("cooked_chicken", 1)),
				actions:   []Action{UseItemAction("cooked_chicken", 1), ShowHealthAction},
			},
			{
				name:      "rest if almost dead",
				condition: LowHealthCondition(50),
				actions:   []Action{RestAction, ShowHealthAction},
			},
			{
				name:      "move to cows place",
				condition: NotCondition(POICondition(CowPOI)),
				actions:   []Action{MovePOIAction(CowPOI)},
			},
			{
				name:      "fight cows",
				condition: AlwaysCondition,
				actions:   []Action{FightAction, ShowInventoryAction, ShowHealthAction},
			},
		},
	}

	slimePlan = Plan{
		name: "farm slime",
		preConditions: []Condition{
			AndCondition(
				MinHealthCondition(100),
				MinLevelCondition(8),
				ChanceCondition(0.8),
				NotCondition(FullInventoryCondition()),
			),
		},
		alternatives: []Alternative{
			{
				name:      "eat food if almost dead",
				condition: AndCondition(LowHealthCondition(100), HasItemCondition("cooked_chicken", 1)),
				actions:   []Action{UseItemAction("cooked_chicken", 1), ShowHealthAction},
			},
			{
				name:      "rest if almost dead",
				condition: LowHealthCondition(50),
				actions:   []Action{RestAction, ShowHealthAction},
			},
			{
				name:      "move to slime place",
				condition: AlwaysCondition,
				actions: []Action{ChooseActionHelper(
					MovePOIAction(GreenSlimePOI),
					MovePOIAction(RedSlimePOI),
					MovePOIAction(BlueSlimePOI),
				)},
			},
			{
				name:      "fight slime",
				condition: AlwaysCondition,
				actions:   []Action{FightAction, ShowInventoryAction, ShowHealthAction},
			},
		},
	}

	cookingPlan = Plan{
		name:          "cook chicken",
		preConditions: []Condition{HasItemCondition("raw_chicken", 20)},
		alternatives: []Alternative{
			{
				name:      "move to kitchen",
				condition: NotCondition(POICondition(CookingPOI)),
				actions:   []Action{MovePOIAction(CookingPOI)},
			},
			{
				name:      "cook",
				condition: AlwaysCondition,
				actions:   []Action{CraftAction("cooked_chicken", 20)},
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

	plans = []Plan{
		cookingPlan,
		slimePlan,
		cowPlan,
		chickenPlan,
		woodPlan,
	}
)

type Plan struct {
	name          string
	preConditions []Condition
	alternatives  []Alternative
}

func (p *Plan) IsExecutable(character, token string) bool {
	charInfo := must(do("characters/"+character, token, nil))
	for _, cond := range p.preConditions {
		if !cond(charInfo) {
			return false
		}
	}
	return true
}

func (p *Plan) Execute(character, token string) {
	fmt.Printf("[%-10s] %s\n", character, p.name)
	charInfo := must(do("characters/"+character, token, nil))
	for _, alt := range p.alternatives {
		if alt.condition(charInfo) {
			for _, action := range alt.actions {
				action.Do(character, token)
			}
			return
		}
	}
}
