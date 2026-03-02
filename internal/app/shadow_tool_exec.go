package app

import (
	"attention-crm/internal/agent"
	"attention-crm/internal/tenantdb"
	"encoding/json"
	"errors"
	"strings"
)

type cilAppendINVArgs struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Trigger string  `json:"trigger"`
	Because string  `json:"because"`
	IfNot   string  `json:"if_not"`
	Scope   string  `json:"scope"`
	Seen    *int64  `json:"seen"`
	Last    *string `json:"last"`
	Ex      *string `json:"ex"`
	Rev     string  `json:"rev"`
	Src     string  `json:"src"`
}

type notesAppendArgs struct {
	Text string `json:"text"`
}

type notesSearchArgs struct {
	Query string `json:"query"`
	Limit *int   `json:"limit"`
}

type shadowToolExecResult struct {
	Terminal bool
	// Output is returned to the model as a tool output message content.
	Output string
}

func executeShadowToolCall(db *tenantdb.Store, agentKey string, actorUserID int64, call agent.FunctionCall) (shadowToolExecResult, error) {
	if strings.TrimSpace(strings.ToLower(call.Type)) != "function_call" {
		return shadowToolExecResult{}, errors.New("not a function_call")
	}

	switch strings.TrimSpace(call.Name) {
	case "ui.no_action":
		return shadowToolExecResult{Terminal: true, Output: "no_action"}, nil

	case "ui.message":
		_ = applyUIFunctionCalls(db, actorUserID, []agent.FunctionCall{call})
		return shadowToolExecResult{Terminal: true, Output: "ok"}, nil

	case "cil.append_inv":
		argsJSON, err := call.NormalizedArguments()
		if err != nil {
			return shadowToolExecResult{}, err
		}
		var args cilAppendINVArgs
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return shadowToolExecResult{}, err
		}
		_, err = db.AppendCILInvariant(agentKey, tenantdb.CILInvariantInput{
			InvID:   args.ID,
			Name:    args.Name,
			Trigger: args.Trigger,
			Because: args.Because,
			IfNot:   args.IfNot,
			Scope:   args.Scope,
			Seen:    args.Seen,
			Last:    args.Last,
			Ex:      args.Ex,
			Rev:     args.Rev,
			Src:     args.Src,
		}, nil)
		if err != nil {
			return shadowToolExecResult{}, err
		}
		return shadowToolExecResult{Terminal: false, Output: "ok"}, nil

	case "notes.append":
		argsJSON, err := call.NormalizedArguments()
		if err != nil {
			return shadowToolExecResult{}, err
		}
		var args notesAppendArgs
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return shadowToolExecResult{}, err
		}
		if _, err := db.AppendAgentNote(agentKey, args.Text); err != nil {
			return shadowToolExecResult{}, err
		}
		return shadowToolExecResult{Terminal: false, Output: "ok"}, nil

	case "notes.search":
		argsJSON, err := call.NormalizedArguments()
		if err != nil {
			return shadowToolExecResult{}, err
		}
		var args notesSearchArgs
		if err := json.Unmarshal(argsJSON, &args); err != nil {
			return shadowToolExecResult{}, err
		}
		limit := 20
		if args.Limit != nil && *args.Limit > 0 && *args.Limit <= 200 {
			limit = *args.Limit
		}
		notes, err := db.SearchAgentNotes(agentKey, args.Query, limit)
		if err != nil {
			return shadowToolExecResult{}, err
		}
		out, _ := json.Marshal(map[string]any{
			"query":   args.Query,
			"results": notes,
		})
		return shadowToolExecResult{Terminal: false, Output: string(out)}, nil
	}

	return shadowToolExecResult{}, errors.New("unknown tool")
}
