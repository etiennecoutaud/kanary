FROM haproxy:latest

RUN apt-get update && apt-get install rsyslog -y && \
    sed -i 's/#$ModLoad imudp/$ModLoad imudp/g' /etc/rsyslog.conf && \
    sed -i 's/#$UDPServerRun 514/$UDPServerRun 514/g' /etc/rsyslog.conf

ADD haproxy.conf /etc/rsyslog.d
COPY docker-entrypoint.sh /

EXPOSE 80 443

ENTRYPOINT ["/docker-entrypoint.sh"]
