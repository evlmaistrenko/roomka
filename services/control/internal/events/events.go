// Package events is a single-process, in-memory publish/subscribe hub. It fans
// domain events (RBAC changes, stream config/status changes) out to the GraphQL
// subscription resolvers that drive the UI's realtime views. One process, one
// container — so no external broker is needed.
package events

import (
	"context"
	"strconv"
	"sync"
)

// subscriberBufferSize bounds how many undelivered events a single subscriber
// may queue. When the buffer is full, Publish drops the oldest event to make
// room for the newest — these events carry live state, so the newest wins.
const subscriberBufferSize = 16

// Event is a payload published to a topic. The concrete type of Payload is
// agreed per topic between publisher and subscriber.
type Event struct {
	Topic   string
	Payload any
}

// Bus routes events from publishers to per-subscriber buffered channels.
type Bus struct {
	mutex       sync.Mutex
	subscribers map[string]map[int]chan Event
	nextID      int
}

// NewBus returns an empty Bus ready to use.
func NewBus() *Bus {
	return &Bus{subscribers: make(map[string]map[int]chan Event)}
}

// Subscribe returns a channel of events published to topic. The subscription is
// removed and its channel closed when ctx is cancelled, so a subscription lives
// exactly as long as the GraphQL operation that opened it.
func (b *Bus) Subscribe(ctx context.Context, topic string) <-chan Event {
	channel := make(chan Event, subscriberBufferSize)

	b.mutex.Lock()
	id := b.nextID
	b.nextID++
	topicSubscribers, ok := b.subscribers[topic]
	if !ok {
		topicSubscribers = make(map[int]chan Event)
		b.subscribers[topic] = topicSubscribers
	}
	topicSubscribers[id] = channel
	b.mutex.Unlock()

	go func() {
		<-ctx.Done()
		b.mutex.Lock()
		if remaining, ok := b.subscribers[topic]; ok {
			delete(remaining, id)
			if len(remaining) == 0 {
				delete(b.subscribers, topic)
			}
		}
		b.mutex.Unlock()
		close(channel)
	}()

	return channel
}

// Publish delivers payload to every current subscriber of topic. It never blocks:
// a subscriber whose buffer is full has its oldest queued event discarded to make
// room. Removal and delivery are serialized by the same mutex, so a channel is
// never sent to after Subscribe's cleanup has removed it from the map.
func (b *Bus) Publish(topic string, payload any) {
	event := Event{Topic: topic, Payload: payload}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	for _, channel := range b.subscribers[topic] {
		if trySend(channel, event) {
			continue
		}
		// Buffer full: drop the oldest queued event, then enqueue the newest.
		select {
		case <-channel:
		default:
		}
		trySend(channel, event)
	}
}

func trySend(channel chan Event, event Event) bool {
	select {
	case channel <- event:
		return true
	default:
		return false
	}
}

// The topics the GraphQL subscriptions use. They are functions rather than
// formatted strings at each call site so that a publisher and a subscriber
// cannot disagree about a topic's name.
//
// Two of them carry state and two carry endings:
//
//   - ViewerTopic and SessionsTopic say "re-read and emit again"
//   - AccessTopic and SessionTopic say "this subscription is over", which is the
//     schema's rule that any revocation or change of permissions closes every
//     subscription on the affected session
const usersTopic = "users"

// UsersTopic is published when the set of users, or anything about one of them,
// changes. Subscribers refetch; the event carries no payload because the
// subscription field does not either.
func UsersTopic() string { return usersTopic }

// ViewerTopic is published when a user's own view of themselves changes:
// profile, permissions, preferences, or the session they are on.
func ViewerTopic(userID int64) string { return "viewer:" + itoa(userID) }

// SessionsTopic is published when a user's session list changes.
func SessionsTopic(userID int64) string { return "sessions:" + itoa(userID) }

// AccessTopic is published when what a user may do changes, and ends every
// subscription that user has open, on every session.
func AccessTopic(userID int64) string { return "access:" + itoa(userID) }

// SessionTopic is published when one session ends, and ends the subscriptions
// running on it.
func SessionTopic(sessionID int64) string { return "session:" + itoa(sessionID) }

func itoa(id int64) string { return strconv.FormatInt(id, 10) }
