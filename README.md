## Overview
This is a simple go tool to listen to an MQTT broker for events from frigate and fire off emails.
It is an attempt at making frigate more useful without having to also run home assistant.

go-frigate-email connects to an MQTT broker and waits for frigate/event messages.  When it gets one, it access the frigate API to download the snapshot from the event.  Once the snapshot is downloaded, an email is sent, including the snapshot as an attachment.

## Prerequisites
- Running [Frigate](https://frigate.video/) installation with detection enabled.  
- Running MQTT broker where frigate is configured to publish.

## Assumptions
For better or worse this assumes:
- Authenticaiton is enabled on your MQTT server (see config.yaml.sample).
- Mailgun API will be used to send emails. 

## Usage
- Clone it.
- Build it with `go build`.
- Run `go-frigate-email`.
You can change the config locaiton with --config.
```
Connects to mqtt and sends messages to a specified email address.

Usage:
  go-frigate-email [flags]

Flags:
  -c, --config string   Configuration file path. (default "config.yaml")
  -h, --help            help for go-frigate-email
```

## Development todos
- Add rules to config
  - Match cameras.
  - Match event types.
  - Match object types.
  - Support complex mapping of events to users (emails).
- Add hold off to prevent spam.
