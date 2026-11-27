# F3 range: unused range variables are reported

<!-- go-check: func() -->
```go
for k, v := range []int{1} {
}
```
