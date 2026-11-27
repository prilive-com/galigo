# F2 no body, local used: the missing body is reported

<!-- go-check: decls -->
```go
func f() {
	unused := 0
	_ = unused
}

func noBody()
```
