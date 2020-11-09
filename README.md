# go-copy-dir-safe
Copy a directory into a target without fear of corruption

To run

```
# build
go build -o copy main.go

# run
./copy -src="test/data/source" -dst="test/data/target"
```

To run tests 
```
go test ./
```

On windows

```
# build
go build -o copy.exe main.go

# run
./copy.exe -src="test/data/source" -dst="test/data/target"
```
