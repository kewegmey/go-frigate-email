package frigate_email

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/mailgun/mailgun-go"
	"gopkg.in/yaml.v2"
)

type Conf struct {
	MqttBroker    string `yaml:"mqttBroker"`
	MqttUsername  string `yaml:"mqttUsername"`
	MqttPassword  string `yaml:"mqttPassword"`
	MailgunDomain string `yaml:"mailgunDomain"`
	MailgunAPIKey string `yaml:"mailgunAPIKey"`
	FrigateURL    string `yaml:"frigateURL"`
	EmailFrom     string `yaml:"emailFrom"`
	EmailSubject  string `yaml:"emailSubject"`
	EmailBody     string `yaml:"emailBody"`
	EmailTo       string `yaml:"emailTo"`
}

type Event struct {
	Type   string `json:"type"`
	Before State  `json:"before"`
	After  State  `json:"after"`
}

type State struct {
	ID                string                 `json:"id"`
	Camera            string                 `json:"camera"`
	FrameTime         float64                `json:"frame_time"`
	SnapshotTime      float64                `json:"snapshot_time"`
	Label             string                 `json:"label"`
	SubLabel          []interface{}          `json:"sub_label"`
	TopScore          float64                `json:"top_score"`
	FalsePositive     bool                   `json:"false_positive"`
	StartTime         float64                `json:"start_time"`
	EndTime           interface{}            `json:"end_time"`
	Score             float64                `json:"score"`
	Box               []int                  `json:"box"`
	Area              int                    `json:"area"`
	Ratio             float64                `json:"ratio"`
	Region            []int                  `json:"region"`
	CurrentZones      []string               `json:"current_zones"`
	EnteredZones      []string               `json:"entered_zones"`
	Thumbnail         interface{}            `json:"thumbnail"`
	HasSnapshot       bool                   `json:"has_snapshot"`
	HasClip           bool                   `json:"has_clip"`
	Stationary        bool                   `json:"stationary"`
	MotionlessCount   int                    `json:"motionless_count"`
	PositionChanges   int                    `json:"position_changes"`
	Attributes        map[string]float64     `json:"attributes"`
	CurrentAttributes []CurrentAttributeData `json:"current_attributes"`
}

type CurrentAttributeData struct {
	Label string  `json:"label"`
	Box   []int   `json:"box"`
	Score float64 `json:"score"`
}

func prettyPrint(event Event) {
	bytes, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		log.Println("Error pretty printing Event:", err)
		return
	}
	log.Println(string(bytes))
}

func readConfig(filename string) (Conf, error) {
	file, err := os.Open(filename)
	if err != nil {
		return Conf{}, err
	}
	defer file.Close()

	var conf Conf
	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&conf)
	if err != nil {
		return Conf{}, err
	}

	return conf, nil
}

func Start(configPath string) {
	// Load the configuration
	conf, err := readConfig(configPath)
	if err != nil {
		log.Println("Error reading configuration file:", err)
		log.Fatal(err)
	}

	// MQTT client options
	log.Println("MQTT Broker:", conf.MqttBroker)
	opts := mqtt.NewClientOptions().AddBroker(conf.MqttBroker)
	opts.SetDefaultPublishHandler(createMessagePubHandler(conf))

	// Set MQTT authentication
	opts.SetUsername(conf.MqttUsername)
	opts.SetPassword(conf.MqttPassword)

	// Create MQTT client
	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	//Subscribe to MQTT topics
	if token := c.Subscribe("frigate/events", 0, nil); token.Wait() && token.Error() != nil {
		log.Println(token.Error())
		os.Exit(1)
	}

	// Keep the application running
	select {}
}

// messagePubHandler is the MQTT message handler
func createMessagePubHandler(conf Conf) mqtt.MessageHandler {
	return func(client mqtt.Client, msg mqtt.Message) {
		log.Printf("Received message from topic: %s\n", msg.Topic())

		// Process the message
		if msg.Topic() == "frigate/events" {
			var event Event
			if err := json.Unmarshal(msg.Payload(), &event); err != nil {
				log.Fatal(err)
			}
			processEvent(event)
			processSnapshot(event, conf)
		}
	}
}

// processEvent processes the MQTT event message
func processEvent(event Event) {
	prettyPrint(event)
}

// processSnapshot processes the MQTT snapshot message and sends an email
func processSnapshot(event Event, conf Conf) {
	// Go get the snapshot from the API.
	//if event.Type == "start" && event.After.HasSnapshot && event.After.Label != "car" {
	if event.Type == "end" && event.After.HasSnapshot {
		url := fmt.Sprintf("%s/api/events/%s/snapshot.jpg?bbox=1&crop=1", conf.FrigateURL, event.After.ID)

		response, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		defer response.Body.Close()

		// Create a temporary file
		out, err := os.CreateTemp("", "snapshot-*.jpg")
		if err != nil {
			log.Fatal(err)
		}
		defer out.Close()
		defer os.Remove(out.Name()) //cleanup

		// Write the body to file
		_, err = io.Copy(out, response.Body)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Println("Saved snapshot to:", out.Name())

		mg := mailgun.NewMailgun(conf.MailgunDomain, conf.MailgunAPIKey)

		sender := conf.EmailFrom
		subject := conf.EmailSubject
		body := conf.EmailBody
		recipient := conf.EmailTo

		// Create a new email message
		msg := mg.NewMessage(sender, subject, body, recipient)

		// Attach the image to the email
		msg.AddAttachment(out.Name())

		// Send the email
		resp, id, err := mg.Send(msg)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("ID: %s Resp: %s\n", id, resp)
	}

}
