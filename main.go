package main

import (
	"bufio"
	"fmt"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
)

var (
	defaultPS      = "> "
	defaultDbFile  = ".sshman/db.json"
	defaultKeyFile = ".ssh/id_rsa"
	usage          = "ssh box"
	root, _        = os.UserHomeDir()
	dbFile         string
	db             *DB

	modeFlag      = cli.StringFlag{Name: "mode", Value: "ssh"}
	databaseFlag = cli.StringFlag{Name: "db", Value: "", Usage: "database"}
	nameFlag     = cli.StringFlag{Name: "name", Value: "", Usage: "name"}
	hostFlag     = cli.StringFlag{Name: "host", Value: "", Usage: "host"}
	portFlag     = cli.IntFlag{Name: "port", Value: 22, Usage: "port"}
	userFlag     = cli.StringFlag{Name: "user", Value: "root", Usage: "user"}
	passFlag     = cli.StringFlag{Name: "pass", Value: "", Usage: "password"}
	keyFlag      = cli.StringFlag{Name: "key", Value: "", Usage: "key file"}
	commentFlag  = cli.StringFlag{Name: "comment", Usage: "comment"}
	forceFlag    = cli.BoolFlag{Name: "force", Usage: "force"}
)

func init() {
	dbFile = os.Getenv("SSH_MAN_DATABASE")
	if dbFile == "" {
		dbFile = path.Join(root, defaultDbFile)
	}
}

func main() {
	app := cli.NewApp()
	app.Flags = append(app.Flags, nameFlag)
	app.Flags = append(app.Flags, databaseFlag)
	app.Flags = append(app.Flags, modeFlag)
	app.Usage = usage
	app.Commands = []cli.Command{
		{
			Name:    "list",
			Aliases: []string{"l"},
			Action:  list,
		},
		{
			Name:   "add",
			Action: add,
			Flags: []cli.Flag{
				nameFlag,
				hostFlag,
				portFlag,
				userFlag,
				passFlag,
				keyFlag,
				commentFlag,
				forceFlag,
			},
		},
		{
			Name:   "del",
			Action: del,
			Flags: []cli.Flag{
				nameFlag,
			},
		},
	}
	app.Before = before
	app.Action = login
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println(err)
	}
}

func before(c *cli.Context) error {
	f := c.String(databaseFlag.Name)
	if f == "" {
		f = dbFile
	}
	var err error
	db, err = OpenDB(f)
	if err != nil {
		return err
	}
	return nil
}

func login(c *cli.Context) error {
	ce := func(err error, msg string) {
		if err != nil {
			fmt.Printf("%s error: %v\n", msg, err)
			os.Exit(2)
		}
	}
	var (
		node *Node
		ok   bool
	)
	name := c.String(nameFlag.Name)
	if name == "" {
		nodes := db.All()
		printNodes(nodes)
		reader := bufio.NewReader(os.Stdin)
		fmt.Println(">>> choose host by name/id")
		for {
			fmt.Print(defaultPS)
			text, _ := reader.ReadString('\n')
			// convert CRLF to LF
			text = strings.Replace(text, "\n", "", -1)
			if len(text) == 0 {
				continue
			}
			if text == "exit" || text == "quit" || text == "q" {
				return nil
			}
			idx, err := strconv.Atoi(text)
			if err != nil {
				node, ok = db.GetByName(text)
				if !ok {
					fmt.Printf("error: [%s] not found\n", text)
				} else {
					goto goon
				}
			} else {
				if idx > len(nodes)-1 {
					fmt.Printf("error: (%d) not found\n", idx)
					continue
				}
				node = nodes[idx]
				if node != nil {
					goto goon
				}
			}
		}
	} else {
		node, ok = db.GetByName(name)
		if !ok {
			return fmt.Errorf("%s not found", name)
		}
	}
goon:
	var (
		client      *ssh.Client
		err         error
		authMethods []ssh.AuthMethod
	)

	args := []string{
		fmt.Sprintf("%s@%s", node.User, node.Host),
		"-p", fmt.Sprintf("%d", node.Port),
	}

	if node.Pass != "" && node.Key != "" {
		pemBytes, _ := ioutil.ReadFile(node.Key)
		signer, err := ssh.ParsePrivateKeyWithPassphrase(pemBytes, []byte(node.Pass))
		if err != nil {
			log.Panic(err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))

	} else if node.Pass != "" {
		authMethods = append(authMethods, ssh.Password(node.Pass))
		args = append(args, "-p")
		args = append(args, node.Pass)

	} else {
		key := node.Key
		if node.Key == "" {
			key = path.Join(root, defaultKeyFile)
		}
		pemBytes, _ := ioutil.ReadFile(key)
		signer, err := ssh.ParsePrivateKey(pemBytes)
		if err != nil {
			log.Panic(err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))

		args = append(args, "-i")
		args = append(args, key)
	}

	mode := c.String(modeFlag.Name)
	if mode == "ssh" {
		cmd := exec.Command( "ssh", args...)
		cmd.Stdout = os.Stdout
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println(err)
		}
	} else {
		client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", node.Host, node.Port), &ssh.ClientConfig{
			User:            node.User,
			Auth:            authMethods,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		})

		ce(err, "dial")
		session, err := client.NewSession()
		ce(err, "new session")
		defer session.Close()
		session.Stdout = os.Stdout
		session.Stderr = os.Stderr
		session.Stdin = os.Stdin
		modes := ssh.TerminalModes{
			ssh.ECHO:          0,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		err = session.RequestPty("linux", 32, 160, modes)
		ce(err, "request pty")
		err = session.Shell()
		ce(err, "start shell")
		err = session.Wait()
		ce(err, "return")
	}
	return nil
}

func printNodes(nodes []*Node) {
	ns := NewNodes(nodes)
	sort.Sort(ns)
	for i, v := range ns.data {
		fmt.Printf("%-4s [ %-20s ] %-21s\t%s\n", fmt.Sprintf("(%d)", i), v.Name, fmt.Sprintf("%s:%d", v.Host, v.Port), v.Comment)
	}
}

func list(c *cli.Context) error {
	printNodes(db.All())
	return nil
}

func add(c *cli.Context) error {
	name := c.String(nameFlag.Name)
	host := c.String(hostFlag.Name)
	port := c.Int(portFlag.Name)
	user := c.String(userFlag.Name)
	pass := c.String(passFlag.Name)
	key := c.String(keyFlag.Name)
	force := c.Bool(forceFlag.Name)
	comment := c.String(commentFlag.Name)

	fmt.Printf("name: %s\nhost: %s\nport: %d\nuser: %s\npass: %s\nkey: %s\nforce: %v\ncomment: %s\n", name, host, port, user, pass, key, force, comment)

	if name == "" || host == "" || user == "" || port <= 0 || port > 65535 {
		return fmt.Errorf("params error")
	}

	node := &Node{
		Name:    name,
		Host:    host,
		Port:    port,
		User:    user,
		Pass:    pass,
		Key:     key,
		Comment: comment,
	}
	_, ok := db.Get(node.GetId())
	if !force && ok {
		return fmt.Errorf("%s exists", node.GetId())
	}
	db.Save(node)
	return nil
}

func del(c *cli.Context) error {
	name := c.String(nameFlag.Name)
	if name == "" {
		return fmt.Errorf("params error")
	}
	db.Del(name)
	return nil
}
