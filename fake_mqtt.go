package wbgo

import (
	"fmt"
	"strings"
	"testing"
)

func topicPartsMatch(pattern []string, topic []string) bool {
	if len(pattern) == 0 {
		return len(topic) == 0
	}

	if pattern[0] == "#" {
		return true
	}

	return len(topic) > 0 &&
		(pattern[0] == "+" || (pattern[0] == topic[0])) &&
		topicPartsMatch(pattern[1:], topic[1:])
}

func topicMatch(pattern string, topic string) bool {
	return topicPartsMatch(strings.Split(pattern, "/"), strings.Split(topic, "/"))
}

func FormatMQTTMessage(message MQTTMessage) string {
	suffix := ""
	if message.Retained {
		suffix = ", retained"
	}
	return fmt.Sprintf("[%s] (QoS %d%s)",
		string(message.Payload), message.QoS, suffix)
}

type SubscriptionList []*FakeMQTTClient
type SubscriptionMap map[string]SubscriptionList

type FakeMQTTBroker struct {
	Recorder
	subscriptions SubscriptionMap
}

func NewFakeMQTTBroker (t *testing.T) (broker *FakeMQTTBroker) {
	broker = &FakeMQTTBroker{subscriptions: make(SubscriptionMap)}
	broker.InitRecorder(t)
	return
}

func (broker *FakeMQTTBroker) Publish(origin string, message MQTTMessage) {
	broker.Rec("%s -> %s: %s", origin, message.Topic, FormatMQTTMessage(message))
	for pattern, subs := range broker.subscriptions {
		if !topicMatch(pattern, message.Topic) {
			continue
		}
		for _, client := range subs {
			client.receive(message)
		}
	}
}

func (broker *FakeMQTTBroker) Subscribe(client *FakeMQTTClient, topic string) {
	broker.Rec("Subscribe -- %s: %s", client.id, topic)
	subs, found := broker.subscriptions[topic]
	if (!found) {
		broker.subscriptions[topic] = SubscriptionList{ client }
	} else {
		for _, c := range subs {
			if c == client {
				return
			}
		}
		broker.subscriptions[topic] = append(subs, client)
	}
}

func (broker *FakeMQTTBroker) Unsubscribe(client *FakeMQTTClient, topic string) {
	broker.Rec("Unsubscribe -- %s: %s", client.id, topic)
	subs, found := broker.subscriptions[topic]
	if (!found) {
		return
	} else {
		newSubs := make(SubscriptionList, 0, len(subs))
		for _, c := range subs {
			if c != client {
				newSubs = append(newSubs, c)
			}
		}
		broker.subscriptions[topic] = newSubs
	}
}

func (broker *FakeMQTTBroker) MakeClient(id string) *FakeMQTTClient {
	return &FakeMQTTClient{id, false, broker, make(map[string][]MQTTMessageHandler)}
}

type FakeMQTTClient struct {
	id string
	started bool
	broker *FakeMQTTBroker
	callbackMap map[string][]MQTTMessageHandler
}

func (client *FakeMQTTClient) receive(message MQTTMessage) {
	for topic, handlers := range client.callbackMap {
		if !topicMatch(topic, message.Topic) {
			continue
		}
		for _, handler := range handlers {
			handler(message)
		}
	}
}

func (client *FakeMQTTClient) Start() {
	if (client.started) {
		client.broker.T().Fatalf("%s: client already started", client.id)
	}
	client.started = true
}

func (client *FakeMQTTClient) Stop() {
	client.ensureStarted()
	client.started = false
	client.broker.Rec("stop: %s", client.id)
}

func (client *FakeMQTTClient) ensureStarted() {
	if (!client.started) {
		client.broker.T().Fatalf("%s: client not started", client.id)
	}
}

func (client *FakeMQTTClient) Publish(message MQTTMessage) {
	client.ensureStarted()
	client.broker.Publish(client.id, message)
}

func (client *FakeMQTTClient) Subscribe(callback MQTTMessageHandler, topics... string) {
	client.ensureStarted()
	for _, topic := range topics {
		client.broker.Subscribe(client, topic)
		handlerList, found := client.callbackMap[topic]
		if found {
			client.callbackMap[topic] = append(handlerList, callback)
		} else {
			client.callbackMap[topic] = []MQTTMessageHandler{callback}
		}
	}
}

func (client *FakeMQTTClient) Unsubscribe(topics... string) {
	client.ensureStarted()
	for _, topic := range topics {
		client.broker.Unsubscribe(client, topic)
		delete(client.callbackMap, topic)
	}
}
