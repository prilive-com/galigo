package tg

// DirectMessagesTopic describes a topic of a channel direct messages chat (Bot API 9.2).
// It is the type of Message.DirectMessagesTopic. To answer in the same topic, pass TopicID as the
// DirectMessagesTopicID of a send request. That field is an int64 in most requests; in
// sender.SendLivePhotoRequest and sender.SendRichMessageRequest it is still an int.
type DirectMessagesTopic struct {
	// TopicID is the topic's unique id. It may need more than 32 bits (at most 52), so it is an int64.
	TopicID int64 `json:"topic_id"`
	// User is the user that created the topic. The spec marks it optional and says it is currently always present.
	User *User `json:"user,omitempty"`
}
