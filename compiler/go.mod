module github.com/goplus/llgo/compiler

go 1.23.0

toolchain go1.23.6

require (
	github.com/goplus/gogen v1.16.6
	github.com/goplus/llgo v0.9.9
	github.com/goplus/llgo/runtime v0.0.0-00010101000000-000000000000
	github.com/goplus/llvm v0.8.1
	github.com/goplus/mod v0.13.17
	github.com/qiniu/x v1.13.12
	golang.org/x/mod v0.24.0
	golang.org/x/tools v0.30.0
)

require golang.org/x/sync v0.11.0 // indirect

replace github.com/goplus/llgo => ../

replace github.com/goplus/llgo/runtime => ../runtime
