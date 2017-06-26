FROM prom/prometheus:v1.7.1

COPY prometheus.yml /etc/prometheus/prometheus.yml

CMD [ "-config.file=/etc/prometheus/prometheus.yml", "-storage.local.engine=none" ]
