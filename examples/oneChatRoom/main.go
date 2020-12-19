package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/kazhmir/gna"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	addr   = flag.String("serve", "", "Hosting address (for you to host your own room).")
	server = flag.String("conn", "", "Server address (the room you're trying to connect).")
	name   = flag.String("name", "idiot", "Client name.")
)

var (
	cli  *gna.Client
	done = make(chan struct{})
	term = bufio.NewReader(os.Stdin)
)

func main() {
	flag.Parse()
	gna.Register(srAuth{}, cliAuth{}, Message{})
	if *addr == "" && *server == "" {
		fmt.Println("You need to either host or connect. Use '-serve <addr>' to host or '-conn <addr>' to connect.")
		return
	}
	if *addr != "" {
		gna.SetReadTimeout(60 * time.Second)
		gna.SetWriteTimeout(60 * time.Second)
		go func() {
			if err := gna.RunServer(*addr, &Room{Users: make(map[uint64]string, 64)}); err != nil {
				log.Fatal(err)
			}
			close(done)
		}()
	} else {
		close(done)
	}
	if *server != "" {
		fmt.Printf("Dialing %v as '%v'.\n", *server, *name)
		cli = Connect(*server, *name)
		cli.SetTimeout(60 * time.Second)
		go CliUpdate()
		ClientLoop()
	}
	<-done
	fmt.Println("Exited.")
}

func CliUpdate() {
	ticker := time.NewTicker(50 * time.Millisecond)
	for {
		<-ticker.C
		data := cli.RecvBatch() // err is aways nil with cli.Start()
		if err := cli.Error(); err != nil {
			log.Fatalf("Recv Error: %v\n", err)
		}
		if len(data) == 0 {
			continue
		}
		for _, dt := range data {
			switch v := dt.(type) {
			case Message:
				fmt.Printf("%v: %v\n", v.Username, v.Data)
			default:
				fmt.Printf("\n%#v, %T\n", dt, dt)
			}
		}
	}
}

func ClientLoop() {
	for {
		msg, err := term.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			continue
		}
		cli.Send(strings.ReplaceAll(msg, "\n", ""))
		if err := cli.Error(); err != nil {
			log.Fatalf("Send Error: %v\n", err)
		}
	}
}

func Connect(addr, name string) *gna.Client {
	cli, err := gna.Dial(addr)
	if err != nil {
		log.Fatal(err)
	}
	err = cli.Send(cliAuth{name})
	if err != nil {
		log.Fatal(err)
	}
	dts, err := cli.Recv()
	if v, ok := dts.(srAuth); ok {
		fmt.Printf("UserID: %v\n", v.UserID)
	}
	cli.Start()
	return cli
}

type Room struct {
	Users map[uint64]string
	gna.Net
	mu sync.Mutex
}

func (r *Room) Update() {
	dt := r.GetData()
	for _, input := range dt {
		if s, ok := input.Data.(string); ok {
			r.mu.Lock()
			r.Dispatch(r.Players, Message{Username: r.Users[input.P.ID], Data: s})
			r.mu.Unlock()
		}
	}
}

func (r *Room) Auth(p *gna.Player) {
	dt, err := p.Recv()
	if err != nil {
		log.Println(err)
		p.Close()
		return
	}
	if v, ok := dt.(cliAuth); ok {
		r.mu.Lock()
		r.Users[p.ID] = v.Name
		fmt.Printf("%v (ID: %v) Connected.\n", v.Name, p.ID)
		r.Dispatch(r.Players, Message{Username: "server", Data: v.Name + " Connected."})
		r.mu.Unlock()
	}
	r.mu.Lock()
	err = p.Send(srAuth{UserID: p.ID})
	r.mu.Unlock()
	if err != nil {
		log.Println(err)
		p.Close()
	}
}

func (r *Room) Disconn(p *gna.Player) {
	fmt.Printf("%v (ID: %v) Disconnected. Reason: %v\n", r.Users[p.ID], p.ID, p.Error())
	r.Dispatch(r.Players, Message{Username: "server", Data: r.Users[p.ID] + " Disconnected."})
	r.mu.Lock()
	delete(r.Users, p.ID)
	r.mu.Unlock()
}

type srAuth struct {
	UserID uint64
}
type cliAuth struct {
	Name string
}

type Message struct {
	Username string
	Data     string
}
