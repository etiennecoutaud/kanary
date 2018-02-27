FROM ubuntu as builder
WORKDIR /home/
RUN apt-get update
RUN apt-get install wget -y
RUN wget https://github.com/prometheus/haproxy_exporter/releases/download/v0.9.0/haproxy_exporter-0.9.0.linux-amd64.tar.gz -O haproxy_exporter
RUN tar -xvf haproxy_exporter

FROM scratch
WORKDIR /root/
COPY --from=builder /home/haproxy_exporter-0.9.0.linux-amd64/haproxy_exporter .
EXPOSE 9101
CMD ["./haproxy_exporter", "--haproxy.scrape-uri=http://localhost:80/monitor;csv"]
