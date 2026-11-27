# F2 no body: the unused local is reported

<!-- go-check: decls -->
```go
func f() {
	unused := 0
}

func noBody()
```
