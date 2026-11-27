# F3 nested: an unused nested result is reported

<!-- go-check: func() -->
```go
import "strconv"

if true {
	msg, err := strconv.Atoi("1")
}
```
