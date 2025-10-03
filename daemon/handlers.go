package daemon

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/ThreeDotsLabs/watermill/message"
)

func SocketTopicProccessor(messages <-chan *message.Message) {
	for msg := range messages {
		ctx := context.Background()
		slog.InfoContext(ctx, "received message: ", slog.String("msg.uuid", msg.UUID))

		var socketMsg SocketMessage
		if err := json.Unmarshal(msg.Payload, &socketMsg); err != nil {
			slog.ErrorContext(ctx, "failed to parse socket message", slog.Any("err", err))
			msg.Nack()
		}

		if socketMsg.Type == SocketMessageTypeSync {
			err := handlePubSubSync(ctx, socketMsg.Payload)
			if err != nil {
				slog.ErrorContext(ctx, "failed to parse socket message", slog.Any("err", err))
				msg.Nack()
			} else {
				msg.Ack()
			}
		}
	}
}
