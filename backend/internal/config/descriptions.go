package config

type Desc struct {
	Key  string `json:"key"`
	Lab  string `json:"label"`
	Text string `json:"text"`
	Type string `json:"type"`
}

func GetDescriptions() []Desc {
	return []Desc{
		{Key: "openclaw_root_dir", Lab: "OpenClaw Root", Text: "Directory where your OpenClaw data is stored", Type: "dir"},
		{Key: "dashboard.host", Lab: "Listen Address", Text: "0.0.0.0 = accessible from network, 127.0.0.1 = local only", Type: "str"},
		{Key: "dashboard.port", Lab: "Port", Text: "Web server port (default 8080)", Type: "int"},
		{Key: "dashboard.theme", Lab: "Theme", Text: "dark or light", Type: "str"},
		{Key: "security.lan_only", Lab: "LAN Only", Text: "Restrict to local network only (recommended)", Type: "bool"},
	}
}
