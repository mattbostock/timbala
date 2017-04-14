FROM prom/prometheus:v1.6.0

COPY prometheus.yml /etc/prometheus/prometheus.yml

CMD [ "-config.file=/etc/prometheus/prometheus.yml", "-storage.local.engine=none" ]
