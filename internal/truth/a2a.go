package truth

import (
	"context"
	"encoding/json"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	"github.com/a2aproject/a2a-go/v2/a2aclient/agentcard"
)

func (r *run) a2a(ctx context.Context, e rawEntry, raw []byte) Endpoint {
	ep := Endpoint{Protocol: "a2a", Identifier: e.Identifier, CardURL: e.URL}
	pc := Check{Entry: e.Identifier, ID: "a2a.card.parse", Rule: "A2A 1.0 AgentCard: name, version, supportedInterfaces required"}
	var card a2a.AgentCard
	switch err := json.Unmarshal(raw, &card); {
	case err != nil:
		pc.Status, pc.Got = Fail, err.Error()
	case card.Name == "" || card.Version == "" || len(card.SupportedInterfaces) == 0:
		pc.Status, pc.Got = Fail, "missing name, version or supportedInterfaces"
	default:
		pc.Status = Pass
	}
	r.add(pc)
	if pc.Status == Fail {
		return ep
	}
	ep.Name, ep.Version, ep.URL = card.Name, card.Version, card.SupportedInterfaces[0].URL

	rule := "A2A 1.0: catalog card should match the agent's own well-known card"
	wk, ok := r.liveCheck(e.Identifier, "a2a.live.card", rule)
	if ok {
		live, err := agentcard.NewResolver(r.opt.HTTP).Resolve(ctx, originOf(ep.URL))
		if err != nil {
			wk.Status, wk.Want, wk.Got = Fail, originOf(ep.URL)+"/.well-known/agent-card.json", err.Error()
		} else {
			wk = compare(wk, card.Name+"@"+card.Version+" "+ep.URL, live.Name+"@"+live.Version+" "+live.SupportedInterfaces[0].URL)
		}
		r.add(wk)
	}

	send, ok := r.liveCheck(e.Identifier, "a2a.live.send", "a2a-go Client.SendMessage on supportedInterfaces[0]")
	if !ok {
		return ep
	}
	client, err := a2aclient.NewFromCard(ctx, &card, a2aclient.WithJSONRPCTransport(r.opt.HTTP))
	if err == nil {
		_, err = client.SendMessage(ctx, &a2a.SendMessageRequest{Message: a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart("ping"))})
	}
	send.Want = ep.URL
	if err != nil {
		send.Status, send.Got = Fail, err.Error()
	} else {
		send.Status, send.Got = Pass, "reply received"
	}
	r.add(send)
	return ep
}
