# pingpong

small services example setup with grpc and different middlewares added to the grpc connection like

* prometheus metrics with histogram
* zap context logging
* opentracing wiht jagger tracer


### jagger 
start jagger all in one
```
docker run -d -p5775:5775/udp -p6831:6831/udp -p6832:6832/udp \
  -p5778:5778 -p16686:16686 -p14268:14268 jaegertracing/all-in-one:latest
```
