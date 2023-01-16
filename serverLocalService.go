package main

import (
	"fmt"
	"github.com/byzk-org/bypt-server/db"
	"github.com/byzk-org/bypt-server/logs"
	"github.com/byzk-org/bypt-server/socket"
	_ "github.com/byzk-org/bypt-server/socket"
	"github.com/kardianos/service"
	"os"
)

const userServiceTemplate = `[Unit]
Description={{.Description}}
ConditionFileIsExecutable={{.Path|cmdEscape}}
{{range $i, $dep := .Dependencies}} 
{{$dep}} {{end}}

[Service]
StartLimitInterval=5
StartLimitBurst=10
ExecStart={{.Path|cmdEscape}}{{range .Arguments}} {{.|cmd}}{{end}}
{{if .ChRoot}}RootDirectory={{.ChRoot|cmd}}{{end}}
{{if .WorkingDirectory}}WorkingDirectory={{.WorkingDirectory|cmdEscape}}{{end}}
{{if .UserName}}User={{.UserName}}{{end}}
{{if .ReloadSignal}}ExecReload=/bin/kill -{{.ReloadSignal}} "$MAINPID"{{end}}
{{if .PIDFile}}PIDFile={{.PIDFile|cmd}}{{end}}
{{if and .LogOutput .HasOutputFileSupport -}}
StandardOutput=file:/var/log/{{.Name}}.out
StandardError=file:/var/log/{{.Name}}.err
{{- end}}
{{if gt .LimitNOFILE -1 }}LimitNOFILE={{.LimitNOFILE}}{{end}}
{{if .Restart}}Restart={{.Restart}}{{end}}
{{if .SuccessExitStatus}}SuccessExitStatus={{.SuccessExitStatus}}{{end}}
RestartSec=120
EnvironmentFile=-/etc/sysconfig/{{.Name}}

[Install]
WantedBy=default.target`

type program struct{}

const bannerText = ` ____  _  _  ____  ____ 
(  _ \( \/ )(  _ \(_  _)
 ) _ < \  /  )___/  )(  
(____/ (__) (__)   (__)
         Version: 2.0.0`

func (p *program) Start(s service.Service) error {
	//go func() {
	//	//http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
	//	//	runtime.GC()
	//	//	fmt.Fprintln(writer, "ok")
	//	//})
	//	log.Println(http.ListenAndServe(":65526", nil))
	//}()
	db.InitDb()
	logs.InitClearLogListener()
	go p.run()
	return nil
}

func (p *program) run() {
	fmt.Println(bannerText)
	fmt.Println()
	socket.ServerRun()
}

func (p *program) Stop(s service.Service) error {
	return nil
}

func main() {
	defer func() { recover() }()
	isUserStart := false
	args := os.Args
	for _, arg := range args {
		if arg == "--user" {
			isUserStart = true
			break
		}
	}

	option := map[string]interface{}{
		"RunAtLoad": true,
	}

	if isUserStart {
		option["UserService"] = true
		option["SystemdScript"] = userServiceTemplate
	}

	svcConfig := &service.Config{
		Name:        "bypt",
		DisplayName: "byzk java appManager center",
		Description: "bypt server",
		Option:      option,
	}

	prg := &program{}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Println("创建服务实例失败 => ", err.Error())
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		err = service.Control(s, os.Args[1])
		if err != nil {
			fmt.Println("执行命令失败 => ", err.Error())
			os.Exit(1)
		}
		return
	}

	err = s.Run()
	if err != nil {
		fmt.Println("启动服务实例失败!")
		os.Exit(1)
	}

}
