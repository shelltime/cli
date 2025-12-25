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
			continue
		}

		var err error
		switch socketMsg.Type {
		case SocketMessageTypeSync:
			err = handlePubSubSync(ctx, socketMsg.Payload)
		case SocketMessageTypeHeartbeat:
			err = handlePubSubHeartbeat(ctx, socketMsg.Payload)
		default:
			slog.ErrorContext(ctx, "unknown socket message type", slog.String("type", string(socketMsg.Type)))
			msg.Nack()
			continue
		}

		if err != nil {
			slog.ErrorContext(ctx, "failed to handle socket message", slog.Any("err", err), slog.String("type", string(socketMsg.Type)))
			msg.Nack()
		} else {
			msg.Ack()
		}
	}
}
