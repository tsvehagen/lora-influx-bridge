package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/influxdata/influxdb/client/v2"
)

const rxTopic = "application/+/node/+/rx"

type loraRxMsg struct {
	AppID    string `json:"applicationID"`
	AppName  string `json:"applicationName"`
	DevName  string `json:"deviceName"`
	DevEUI   string `json:"devEUI"`
	RxInfo   []struct {
		Time string `json:"time"`
		Rssi int    `json:"rssi"`
	} `json:"rxInfo"`
	Data string `json:"data"`
}

func addToInfluxDB(measurement string, tags map[string]string, fields map[string]interface{}, t time.Time) error {
	c, err := client.NewHTTPClient(client.HTTPConfig{
		Addr: os.Getenv("INFLUXDB_SERVER"),
		Username: os.Getenv("INFLUXDB_USERNAME"),
		Password: os.Getenv("INFLUXDB_PASSWORD"),
	})
	if err != nil {
		log.Println("Failed to create influxdb client:", err.Error())
		return err
	}
	defer c.Close()

	// Create a new point batch
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database: os.Getenv("INFLUXDB_DB"),
	})
	if err != nil {
		log.Println("Failed to create batch points:", err.Error())
		return err
	}

	pt, err := client.NewPoint(measurement, tags, fields, t)
	if err != nil {
		log.Println("Failed to add point:", err.Error())
		return err
	}
	bp.AddPoint(pt)

	// Write the batch
	err = c.Write(bp)
	if err != nil {
		log.Println("Failed to write batch:", err.Error())
		return err
	}

	return nil
}

func onLoraRx(c mqtt.Client, m mqtt.Message) {
	var loraMsg loraRxMsg
	var p interface{}

	err := json.Unmarshal(m.Payload(), &loraMsg)
	if err != nil {
		log.Println("Failed to decode json:", err)
		return
	}

	data, err := base64.StdEncoding.DecodeString(loraMsg.Data)
	if err != nil {
		log.Println("Failed to decode base64:", err)
		return
	}

	err = json.Unmarshal(data, &p)
	if err != nil {
		log.Println("Failed to decode json payload:", data)
		return
	}

	tags := map[string]string{
		"app_id":   loraMsg.AppID,
		"dev_name": loraMsg.DevName,
		"dev_eui":  loraMsg.DevEUI,
	}

	t := time.Now()
	fields := p.(map[string]interface{})

	if len(loraMsg.RxInfo) != 0 {
		fields["rssi"] = loraMsg.RxInfo[0].Rssi

		// Gateway might not set time if GPS time is unavailable
		if len(loraMsg.RxInfo[0].Time) != 0 {
			rxTime, err := time.Parse(time.RFC3339Nano, loraMsg.RxInfo[0].Time)
			if err == nil {
				t = rxTime
			}
		}
	}

	addToInfluxDB(loraMsg.AppName, tags, fields, t)
}

func onConnected(c mqtt.Client) {
	log.Printf("Connected to mqtt server")
	if t := c.Subscribe(rxTopic, 2, onLoraRx); t.Wait() && t.Error() != nil {
		fmt.Printf("Failed to subscribe to %s: %v", rxTopic, t.Error())
	}
}

func onConnectionLost(c mqtt.Client, err error) {
	log.Println("Lost connection to mqtt server, will try to reconnect")
}

func newTLSConfig(cafile string) (*tls.Config, error) {
	cert, err := ioutil.ReadFile(cafile)
	if err != nil {
		return nil, err
	}

	certpool := x509.NewCertPool()
	certpool.AppendCertsFromPEM(cert)

	return &tls.Config{
		// RootCAs = certs used to verify server cert.
		RootCAs: certpool,
	}, nil
}

func main() {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(os.Getenv("MQTT_SERVER"))
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetOnConnectHandler(onConnected)
	opts.SetConnectionLostHandler(onConnectionLost)

	cafile := os.Getenv("MQTT_CA_CERT")

	if cafile != "" {
		tlsconfig, err := newTLSConfig(cafile)
		if err != nil {
			log.Fatalf("Failed to load mqtt CA certificate: %v", err.Error())
		} else {
			opts.SetTLSConfig(tlsconfig)
		}
	}

	client := mqtt.NewClient(opts)

	for {
		if token := client.Connect(); token.Wait() && token.Error() != nil {
			log.Println("Failed to connect to mqtt server, will retry in 2s:", token.Error())
			time.Sleep(2 * time.Second)
		} else {
			break
		}
	}

	log.Println("Waiting for messages...")

	sigChan := make(chan os.Signal)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	log.Println("Stopping")
}
