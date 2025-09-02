package frigate_email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"cloud.google.com/go/storage"
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
	BucketName    string `yaml:"bucketName"`
	GCPCredPath   string `yaml:"gcpCredPath"`
	EmailEnabled  bool   `yaml:"emailEnabled"`
	GCPEnabled    bool   `yaml:"gcpEnabled"`
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
	opts.AutoReconnect = true

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
			processClip(event, conf)
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
	if event.Type == "end" && event.After.HasSnapshot && event.After.EndTime != nil {
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

		if conf.GCPEnabled {
			// GCP upload
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", conf.GCPCredPath)
			// Build object path: snapshots/${year}/${month}/${day}/${camera}/${object type}/${id}.jpg
			t := event.After.EndTime
			// Convert float64 timestamp to time.Time
			endTime := int64(t.(float64))
			// Frigate uses unix epoch seconds, so convert to time.Time
			timeObj := time.Unix(endTime, 0)
			year, month, day := timeObj.Date()
			objectName := fmt.Sprintf(
				"%04d/%02d/%02d/%s/%s/%s.jpg",
				year, int(month), day,
				event.After.Camera,
				event.After.Label,
				event.After.ID,
			)
			file, err := os.Open(out.Name())
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()

			ctx := context.Background()
			client, err := storage.NewClient(ctx)
			if err != nil {
				log.Fatal(err)
			}
			defer client.Close()

			wc := client.Bucket(conf.BucketName).Object(objectName).NewWriter(ctx)
			if _, err = io.Copy(wc, file); err != nil { // We should probably just take this from the response.Body instead of reopening the file.
				log.Fatal(err)
			}
			if err := wc.Close(); err != nil {
				log.Fatal(err)
			}
			log.Printf("Uploaded snapshot to GCP bucket: gs://%s/%s\n", conf.BucketName, objectName)
		}
		if conf.EmailEnabled {
			// email
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
}

func processClip(event Event, conf Conf) {
	if event.Type == "end" && event.After.HasClip && event.After.EndTime != nil {
		url := fmt.Sprintf("%s/api/events/%s/clip.mp4", conf.FrigateURL, event.After.ID)

		var validReader io.ReadCloser
		haveClip := false
		for i := 0; i < 20; i++ {
			response, err := http.Get(url)
			if err != nil {
				log.Fatal(err)
			}
			defer response.Body.Close()

			bodyBytes, err := io.ReadAll(response.Body)
			if err != nil {
				log.Fatal(err)
			}
			if len(bodyBytes) == 0 {
				log.Printf("Clip response body is empty: %s", url)
				time.Sleep(300 * time.Millisecond)
				continue
			} else {
				validReader = io.NopCloser(bytes.NewReader(bodyBytes))
				haveClip = true
				break
			}
		}
		if conf.GCPEnabled && haveClip {
			// GCP upload
			os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", conf.GCPCredPath)
			t := event.After.EndTime
			// Convert float64 timestamp to time.Time
			endTime := int64(t.(float64))
			// Frigate uses unix epoch seconds, so convert to time.Time
			timeObj := time.Unix(endTime, 0)
			year, month, day := timeObj.Date()
			objectName := fmt.Sprintf(
				"%04d/%02d/%02d/%s/%s/%s.mp4",
				year, int(month), day,
				event.After.Camera,
				event.After.Label,
				event.After.ID,
			)

			ctx := context.Background()
			client, err := storage.NewClient(ctx)
			if err != nil {
				log.Fatal(err)
			}
			defer client.Close()

			wc := client.Bucket(conf.BucketName).Object(objectName).NewWriter(ctx)
			if _, err = io.Copy(wc, validReader); err != nil {
				log.Fatal(err)
			}
			if err := wc.Close(); err != nil {
				log.Fatal(err)
			}
			log.Printf("Uploaded clip to GCP bucket: gs://%s/%s\n", conf.BucketName, objectName)
		}
	}
}
