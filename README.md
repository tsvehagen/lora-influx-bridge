## lora-influx-bridge

Small go application to get data from [lora-app-server](https://www.loraserver.io/) over mqtt and inserting it into an influx database. It was created as a way for using [grafana](https://grafana.com/) to present the data from LoRa nodes.

The data field is expected to be in json format and it, together with the rssi, will be added as fields to the measurement. The measurement used will be the same as the applicationName and applicationID, deviceName and devEUI will be added as tags.

See docker-compose.yml for an example of how to use this with influx and grafana.
