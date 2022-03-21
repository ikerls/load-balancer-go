# Load Balancer GO

Simple Load Balancer using Round Robin algorithm with active cleaning and passive recovery for unhealthy backends.

## Usage

`default port: 8080`

```bash
go build
# backends list, use commas to separate
load-balancer-go.exe --backends=http://localhost:8081,http://localhost:8082,http://localhost:8083
```
