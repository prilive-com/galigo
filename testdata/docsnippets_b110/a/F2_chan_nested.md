# F2 chan nested: the unused local is reported

<!-- go-check: func() -->
```go
if true {
	unused := 0
}
_ = make(chan [65536]byte)
```
