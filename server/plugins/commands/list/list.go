package main

import (
	"bufio"
	"fmt"
	"math"
	"strings"

	"github.com/kelvinmwinuka/memstore/utils"
)

const (
	OK = "+OK\r\n\n"
)

type Server interface {
	Lock()
	Unlock()
	GetData(key string) interface{}
	SetData(key string, value interface{})
}

type plugin struct {
	name        string
	commands    []string
	description string
}

var Plugin plugin

func (p *plugin) Name() string {
	return p.name
}

func (p *plugin) Commands() []string {
	return p.commands
}

func (p *plugin) Description() string {
	return p.description
}

func (p *plugin) HandleCommand(cmd []string, server interface{}, conn *bufio.Writer) {
	c := strings.ToLower(cmd[0])

	switch {
	case c == "llen":
		handleLLen(cmd, server.(Server), conn)

	case c == "lindex":
		handleLIndex(cmd, server.(Server), conn)

	case c == "lrange":
		handleLRange(cmd, server.(Server), conn)

	case c == "lset":
		handleLSet(cmd, server.(Server), conn)

	case c == "ltrim":
		handleLTrim(cmd, server.(Server), conn)

	case c == "lrem":
		handleLRem(cmd, server.(Server), conn)

	case c == "lmove":
		handleLMove(cmd, server.(Server), conn)

	case utils.Contains[string]([]string{"lpush", "lpushx"}, c):
		handleLPush(cmd, server.(Server), conn)

	case utils.Contains[string]([]string{"rpush", "rpushx"}, c):
		handleRPush(cmd, server.(Server), conn)

	case utils.Contains[string]([]string{"lpop", "rpop"}, c):
		handlePop(cmd, server.(Server), conn)
	}
}

func handleLLen(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 2 {
		conn.Write([]byte("-Error wrong number of args for LLEN command\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LLEN command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	server.Unlock()
	conn.Write([]byte(fmt.Sprintf(":%d\r\n\n", len(list))))
	conn.Flush()
}

func handleLIndex(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 3 {
		conn.Write([]byte("-Error wrong number of args for LINDEX command\r\n\n"))
		conn.Flush()
		return
	}

	index, ok := utils.AdaptType(cmd[2]).(int)

	if !ok {
		conn.Write([]byte("-Error index must be an integer\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LINDEX command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	if !(index >= 0 && index < len(list)) {
		server.Unlock()
		conn.Write([]byte("-Error index must be within list range\r\n\n"))
		conn.Flush()
		return
	}

	server.Unlock()
	conn.Write([]byte(fmt.Sprintf("+%s\r\n\n", list[index])))
	conn.Flush()
}

func handleLRange(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 4 {
		conn.Write([]byte("-Error wrong number of arguments for LRANGE command\r\n\n"))
		conn.Flush()
		return
	}

	start, startOk := utils.AdaptType(cmd[2]).(int)
	end, endOk := utils.AdaptType(cmd[3]).(int)

	if !startOk || !endOk {
		conn.Write([]byte("-Error both start and end indices must be integers\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	server.Unlock()

	if !ok {
		conn.Write([]byte("-Error type cannot be returned with LRANGE command\r\n\n"))
		conn.Flush()
		return
	}

	// Make sure start is within range
	if !(start >= 0 && start < len(list)) {
		conn.Write([]byte("-Error start index not within list range\r\n\n"))
		conn.Flush()
		return
	}

	// Make sure end is within range, or is -1 otherwise
	if !((end >= 0 && end < len(list)) || end == -1) {
		conn.Write([]byte("-Error end index must be within list range or -1\r\n\n"))
		conn.Flush()
		return
	}

	// If end is -1, read list from start to the end of the list
	if end == -1 {
		conn.Write([]byte("*" + fmt.Sprint(len(list)-start) + "\r\n"))
		for i := start; i < len(list); i++ {
			str := fmt.Sprintf("%v", list[i])
			conn.Write([]byte("$" + fmt.Sprint(len(str)) + "\r\n" + str + "\r\n"))
		}
		conn.Write([]byte("\n"))
		conn.Flush()
		return
	}

	// Make sure start and end are not equal to each other
	if start == end {
		conn.Write([]byte("-Error start and end indices cannot be equal equal\r\n\n"))
		conn.Flush()
		return
	}

	// If end is not -1:
	//	1) If end is larger than start, return slice from start -> end
	//	2) If end is smaller than start, return slice from end -> start
	conn.Write([]byte("*" + fmt.Sprint(int(math.Abs(float64(start-end)))+1) + "\r\n"))

	i := start
	j := end + 1
	if start > end {
		j = end - 1
	}

	for i != j {
		str := fmt.Sprintf("%v", list[i])
		conn.Write([]byte("$" + fmt.Sprint(len(str)) + "\r\n" + str + "\r\n"))
		if start < end {
			i++
		} else {
			i--
		}

	}
	conn.Write([]byte("\n"))
	conn.Flush()
}

func handleLSet(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 4 {
		conn.Write([]byte("-Error wrong number of arguments for LSET command\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LSET command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	index, ok := utils.AdaptType(cmd[2]).(int)

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error index must be an integer\r\n\n"))
		conn.Flush()
		return
	}

	if !(index >= 0 && index < len(list)) {
		server.Unlock()
		conn.Write([]byte("-Error index must be within range\r\n\n"))
		conn.Flush()
		return
	}

	list[index] = utils.AdaptType(cmd[3])
	server.SetData(cmd[1], list)
	server.Unlock()

	conn.Write([]byte(OK))
	conn.Flush()
}

func handleLTrim(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 4 {
		conn.Write([]byte("-Error wrong number of args for command LTRIM \r\n\n"))
		conn.Flush()
		return
	}

	start, startOk := utils.AdaptType(cmd[2]).(int)
	end, endOk := utils.AdaptType(cmd[3]).(int)

	if !startOk || !endOk {
		conn.Write([]byte("-Error start and end indices must be integers\r\n\n"))
		conn.Flush()
		return
	}

	if end < start && end != -1 {
		conn.Write([]byte("-Error end index must be greater than start index or -1\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LTRIM command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	if !(start >= 0 && start < len(list)) {
		server.Unlock()
		conn.Write([]byte("-Error start index must be within list boundary\r\n\n"))
		conn.Flush()
		return
	}

	if end == -1 || end > len(list) {
		server.SetData(cmd[1], list[start:])
		server.Unlock()
		conn.Write([]byte(OK))
		conn.Flush()
		return
	}

	server.SetData(cmd[1], list[start:end])
	server.Unlock()
	conn.Write([]byte(OK))
	conn.Flush()
}

func handleLRem(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 4 {
		conn.Write([]byte("-Error wrong number of arguments for LREM command\r\n\n"))
		conn.Flush()
		return
	}

	value := cmd[3]
	count, ok := utils.AdaptType(cmd[2]).(int)

	if !ok {
		conn.Write([]byte("-Error count must be an integer\r\n\n"))
		conn.Flush()
		return
	}

	absoluteCount := math.Abs(float64(count))

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LREM command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	switch {
	default:
		// Count is zero, keep list the same
	case count > 0:
		// Start from the head
		for i := 0; i < len(list); i++ {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	case count < 0:
		// Start from the tail
		for i := len(list) - 1; i >= 0; i-- {
			if absoluteCount == 0 {
				break
			}
			if fmt.Sprintf("%v", list[i]) == value {
				list[i] = nil
				absoluteCount -= 1
			}
		}
	}

	list = utils.Filter[interface{}](list, func(elem interface{}) bool {
		return elem != nil
	})

	server.SetData(cmd[1], list)

	server.Unlock()
	conn.Write([]byte(OK))
	conn.Flush()
}

func handleLMove(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 5 {
		conn.Write([]byte("-Error wrong number of arguments for LMOVE command\r\n\n"))
		conn.Flush()
		return
	}

	whereFrom := strings.ToLower(cmd[3])
	whereTo := strings.ToLower(cmd[4])

	if !utils.Contains[string]([]string{"left", "right"}, whereFrom) || !utils.Contains[string]([]string{"left", "right"}, whereTo) {
		conn.Write([]byte("-Error wherefrom and whereto arguments must be either LEFT or RIGHT\r\n\n"))
		conn.Flush()
		return
	}

	server.Lock()

	source, sourceOk := server.GetData(cmd[1]).([]interface{})
	destination, destinationOk := server.GetData(cmd[2]).([]interface{})

	if !sourceOk || !destinationOk {
		server.Unlock()
		conn.Write([]byte("-Error source and destination must both be lists\r\n\n"))
		conn.Flush()
		return
	}

	switch whereFrom {
	case "left":
		server.SetData(cmd[1], append([]interface{}{}, source[1:]...))
		if whereTo == "left" {
			server.SetData(cmd[2], append(source[0:1], destination...))
		} else if whereTo == "right" {
			server.SetData(cmd[2], append(destination, source[0]))
		}
	case "right":
		server.SetData(cmd[1], append([]interface{}{}, source[:len(source)-1]...))
		if whereTo == "left" {
			server.SetData(cmd[2], append(source[len(source)-1:], destination...))
		} else if whereTo == "right" {
			server.SetData(cmd[2], append(destination, source[len(source)-1]))
		}
	}

	server.Unlock()
	conn.Write([]byte(OK))
	conn.Flush()
}

func handleLPush(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) < 3 {
		conn.Write([]byte(fmt.Sprintf("-Error wrong number of arguments for %s command\r\n\n", strings.ToUpper(cmd[0]))))
		conn.Flush()
		return
	}

	server.Lock()

	newElems := []interface{}{}

	for _, elem := range cmd[2:] {
		newElems = append(newElems, utils.AdaptType(elem))
	}

	currentList := server.GetData(cmd[1])

	if currentList == nil {

		switch strings.ToLower(cmd[0]) {
		default:
			server.SetData(cmd[1], newElems)
			conn.Write([]byte(OK))
		case "lpushx":
			conn.Write([]byte("-Error no list at key\r\n\n"))
		}

		server.Unlock()
		conn.Flush()
		return
	}

	l, ok := currentList.([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error LPUSH command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	server.SetData(cmd[1], append(newElems, l...))
	server.Unlock()

	conn.Write([]byte(OK))
	conn.Flush()
}

func handleRPush(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) < 3 {
		conn.Write([]byte(fmt.Sprintf("-Error wrong number of arguments for %s command\r\n\n", strings.ToUpper(cmd[0]))))
		conn.Flush()
		return
	}

	server.Lock()

	newElems := []interface{}{}

	for _, elem := range cmd[2:] {
		newElems = append(newElems, utils.AdaptType(elem))
	}

	currentList := server.GetData(cmd[1])

	if currentList == nil {
		switch strings.ToLower(cmd[0]) {
		default:
			server.SetData(cmd[1], newElems)
			conn.Write([]byte(OK))
		case "rpushx":
			conn.Write([]byte("-Error no list at key\r\n\n"))
		}

		server.Unlock()
		conn.Flush()
		return
	}

	l, ok := currentList.([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte("-Error RPUSH command on non-list item\r\n\n"))
		conn.Flush()
		return
	}

	server.SetData(cmd[1], append(l, newElems...))
	server.Unlock()

	conn.Write([]byte(OK))
	conn.Flush()
}

func handlePop(cmd []string, server Server, conn *bufio.Writer) {
	if len(cmd) != 2 {
		conn.Write([]byte(fmt.Sprintf("-Error wrong number of args for %s command\r\n\n", strings.ToUpper(cmd[0]))))
		conn.Flush()
		return
	}

	server.Lock()

	list, ok := server.GetData(cmd[1]).([]interface{})

	if !ok {
		server.Unlock()
		conn.Write([]byte(fmt.Sprintf("-Error %s command on non-list item\r\n\n", strings.ToUpper(cmd[0]))))
		conn.Flush()
		return
	}

	switch strings.ToLower(cmd[0]) {
	default:
		server.SetData(cmd[1], list[1:])
		conn.Write([]byte(fmt.Sprintf("+%v\r\n\n", list[0])))
	case "rpop":
		server.SetData(cmd[1], list[:len(list)-1])
		conn.Write([]byte(fmt.Sprintf("+%v\r\n\n", list[len(list)-1])))
	}

	server.Unlock()
	conn.Flush()
}

func init() {
	Plugin.name = "ListCommand"
	Plugin.commands = []string{
		"lpush",  // (LPUSH key value1 [value2]) Prepends one or more values to the beginning of a list, creates the list if it does not exist.
		"lpushx", // (LPUSHX key value) Prepends a value to the beginning of a list only if the list exists.
		"lpop",   // (LPOP key) Removes and returns the first element of a list.
		"llen",   // (LLEN key) Return the length of a list.
		"lrange", // (LRANGE key start end) Return a range of elements between the given indices.
		"lindex", // (LINDEX key index) Gets list element by index.
		"lset",   // (LSET key index value) Sets the value of an element in a list by its index.
		"ltrim",  // (LTRIM key start end) Trims a list to the specified range.
		"lrem",   // (LREM key count value) Remove elements from list.
		"lmove",  // (LMOVE source destination <LEFT | RIGHT> <LEFT | RIGHT> Move element from one list to the other specifying left/right for both lists.
		"rpop",   // (RPOP key) Removes and gets the last element in a list.
		"rpush",  // (RPUSH key value [value2]) Appends one or multiple elements to the end of a list.
		"rpushx", // (RPUSHX key value) Appends an element to the end of a list, only if the list exists.
	}
	Plugin.description = "Handle List commands"
}