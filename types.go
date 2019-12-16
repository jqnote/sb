package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
)

type Node struct{
	Name string `json:"name"`
	Host string `json:"host"`
	Port int `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	Key string `json:"key"`
	Comment string `json:"comment"`
}

func (n *Node) GetId() string {
	id := fmt.Sprintf("%s@%s", n.User, n.Host)
	return id
}

type Nodes struct {
	data []*Node
}

func NewNodes(data []*Node) *Nodes {
	return &Nodes{data: data}
}

var(
	_ sort.Interface = &Nodes{}
)

func (s Nodes) Len() int {
	return len(s.data)
}

func (s Nodes) Less(i, j int) bool {
	return strings.Compare(s.data[i].Name, s.data[j].Name) < 0
}

func (s *Nodes) Swap(i, j int) {
	s.data[i], s.data[j] = s.data[j], s.data[i]
}


type DB struct {
	file string
	data map[string]*Node
}

func OpenDB(file string) (*DB, error) {
	database := make(map[string]*Node)
	data, err := ioutil.ReadFile(file)
	if os.IsNotExist(err) {
		bs, err := json.Marshal(database)
		if err != nil {
			log.Panic(err)
			return nil, err
		}
		if err := ioutil.WriteFile(file, bs, 0755); err != nil {
			log.Panic(err)
			return nil, err
		}
	} else {
		if err := json.Unmarshal(data, &database); err != nil {
			return nil, err
		}
	}
	return &DB{
		file: file,
		data: database,
	}, nil
}

func (d *DB) Save(node *Node) {
	d.data[node.GetId()] = node
	d.save()
}

func (d *DB) All() (list []*Node) {
	for _, v := range d.data {
		list = append(list, v)
	}
	return
}

func (d *DB) Del(name string) {
	l := make(map[string]*Node)
	for k, v := range d.data {
		if v.Name == name {
			continue
		}
		l[k] = v
	}
	d.data = l
	d.save()
}

func (d *DB) Get(id string) (*Node, bool) {
	item, ok := d.data[id]
	return item, ok
}

func (d *DB) GetByName(name string) (*Node, bool) {
	for _, v := range d.data {
		if v.Name == name {
			return v, true
		}
	}
	return nil, false
}

func (d *DB) save() {
	bs, _ := json.Marshal(d.data)
	_ = ioutil.WriteFile(d.file, bs, 0755)
}
