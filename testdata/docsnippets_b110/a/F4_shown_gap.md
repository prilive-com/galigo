# F4 shown gap: a shown import list must be complete

<!-- go-check: func(client *sender.Client) -->
```go
import "github.com/prilive-com/galigo/tg"

_ = client
_ = tg.ParseModeHTML
_ = sender.SendMessageRequest{}
```
