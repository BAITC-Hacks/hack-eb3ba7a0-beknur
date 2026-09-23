package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func objectSchema(props map[string]any) map[string]any {
	required := []string{}
	for k := range props {
		required = append(required, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}
func analysisSchema(x Result) map[string]any {
	str := map[string]any{"type": "string"}
	arr := map[string]any{"type": "array", "items": str}
	districts, decisions := map[string]any{}, map[string]any{}
	for _, d := range x.Scenario.Districts {
		districts[d.Name] = str
	}
	for _, d := range x.Picks {
		decisions[d.ID] = str
	}
	return objectSchema(map[string]any{"summary": str, "what_you_did_right": arr, "what_you_did_wrong": arr, "tradeoffs": arr, "risks": arr, "recommendations": arr, "districts": objectSchema(districts), "decisions": objectSchema(decisions)})
}
func validateAnalysis(v Analysis, x Result) error {
	if strings.TrimSpace(v.Summary) == "" {
		return errors.New("AI вернул пустой итог")
	}
	for _, arr := range [][]string{v.Right, v.Improvements, v.Tradeoffs, v.Risks, v.Recommendations} {
		if len(arr) == 0 {
			return errors.New("AI пропустил обязательный раздел")
		}
		for _, s := range arr {
			if strings.TrimSpace(s) == "" {
				return errors.New("AI вернул пустой пункт")
			}
		}
	}
	if len(v.Districts) != len(x.Scenario.Districts) || len(v.Decisions) != len(x.Picks) {
		return errors.New("AI пропустил район или решение")
	}
	for _, d := range x.Scenario.Districts {
		if strings.TrimSpace(v.Districts[d.Name]) == "" {
			return errors.New("Нет объяснения района")
		}
	}
	for _, p := range x.Picks {
		if strings.TrimSpace(v.Decisions[p.ID]) == "" {
			return errors.New("Нет объяснения решения")
		}
	}
	return nil
}
func (a *App) generate(ctx context.Context, x Result) (Analysis, string, error) {
	var out Analysis
	key := os.Getenv("OPENAI_API_KEY")
	model := os.Getenv("OPENAI_MODEL")
	if key == "" {
		return out, model, errors.New("Задайте OPENAI_API_KEY в .env и перезапустите сервер. Результат игры сохранён")
	}
	if model == "" {
		return out, model, errors.New("Задайте OPENAI_MODEL в .env")
	}
	prompt, _ := files.ReadFile("prompts/post-game.txt")
	x.AI = nil
	x.Error = ""
	facts, _ := json.Marshal(x)
	payload := map[string]any{"model": model, "store": false, "instructions": string(prompt), "input": string(facts), "max_output_tokens": 6500, "text": map[string]any{"format": map[string]any{"type": "json_schema", "name": "post_game_report", "strict": true, "schema": analysisSchema(x)}}}
	body, _ := json.Marshal(payload)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, a.aiURL, bytes.NewReader(body))
	if e != nil {
		return out, model, e
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, e := a.client.Do(req)
	if e != nil {
		return out, model, errors.New("Не удалось дождаться AI. Проверьте соединение и повторите анализ")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, model, fmt.Errorf("OpenAI вернул HTTP %d. Проверьте доступ к модели, ключ и квоту; результат сохранён", resp.StatusCode)
	}
	var parsed struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&parsed); e != nil || parsed.Status != "completed" {
		return out, model, errors.New("AI не завершил ответ. Повторите анализ")
	}
	var text strings.Builder
	for _, o := range parsed.Output {
		if o.Type == "message" {
			for _, c := range o.Content {
				if c.Type == "output_text" {
					text.WriteString(c.Text)
				}
			}
		}
	}
	if text.Len() > 100000 {
		return out, model, errors.New("Слишком большой ответ AI")
	}
	decoder := json.NewDecoder(strings.NewReader(text.String()))
	decoder.DisallowUnknownFields()
	if e = decoder.Decode(&out); e != nil {
		return out, model, errors.New("AI вернул некорректный JSON")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return out, model, errors.New("AI вернул лишние данные")
	}
	return out, model, validateAnalysis(out, x)
}
func (a *App) analyze(id string) {
	a.slots <- struct{}{}
	defer func() { <-a.slots }()
	x, e := read[Result](a.store, "results", id)
	if e != nil || x.Status != "analyzing" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 95*time.Second)
	defer cancel()
	analysis, model, aiError := a.generate(ctx, x)
	e = a.store.DB.Update(func(tx *Tx) error {
		current, e := get[Result](tx, "results", id)
		if e != nil {
			return e
		}
		if current.Status != "analyzing" {
			return nil
		}
		current.Model = model
		if aiError != nil {
			current.Status = "analysis_failed"
			current.Error = aiError.Error()
		} else {
			current.Status = "complete"
			current.Error = ""
			current.AI = &analysis
		}
		if e = put(tx, "results", id, current); e != nil {
			return e
		}
		room, e := get[Room](tx, "rooms", current.RoomID)
		if e != nil {
			return e
		}
		complete := len(room.Submitted) == len(room.Players)
		for _, rid := range room.Submitted {
			v, e := get[Result](tx, "results", rid)
			if e != nil {
				return e
			}
			complete = complete && v.Status == "complete"
		}
		if complete {
			room.Status = "complete"
			return put(tx, "rooms", room.ID, room)
		}
		return nil
	})
	if e != nil {
		log.Printf("Не удалось сохранить AI-отчёт %s: %v", id, e)
	}
}
