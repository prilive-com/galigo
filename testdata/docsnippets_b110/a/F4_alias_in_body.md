# F4 alias in body: the generated alias is not visible to the body

<!-- go-check: func(client *sender.Client) -->
```go
import "github.com/prilive-com/galigo/tg"

_ = client
_ = tg.ParseModeHTML
_ = docp_sender.SendMessageRequest{}
```
