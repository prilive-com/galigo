# F2 chan top: the later error is shown

<!-- go-check: func() -->
```go
unused := 0
_ = make(chan [65536]byte)
```
