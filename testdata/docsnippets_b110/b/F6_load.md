# F6 load error: one bad import stops the build

<!-- go-check: func() -->
```go
import "example.invalid/nope"

_ = nope.X
```

<!-- go-check: func() -->
```go
_ = undefinedName
```
