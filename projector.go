package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	//"github.com/byuoitav/common/pooled"
	"github.com/byuoitav/common/structs"
	"github.com/byuoitav/pooled"
)

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	connected := false
	var address string
	ctx := context.Background()
	var projector Projector

	for !connected {
		fmt.Print("Enter Address: ")
		scanner.Scan()
		address = scanner.Text()

		//make projector object
		projector = Projector{
			Address: address,
		}
		projector.poolInit.Do(func() {
			// create the pool
			projector.pool = pooled.NewPool(45*time.Second, 40*time.Second, getConnection)
		})

		test := processCMD(ctx, projector, "power_status ?")
		test += "      "
		if test[0:6] != "failed" {
			connected = true
		} else {
			fmt.Println(test)
		}
		fmt.Println()
	}

	fmt.Print("Connected to " + address + " | Status: ")
	fmt.Println(processCMD(ctx, projector, "power_status ?"))
	fmt.Println("")

	//scan for input
	for {
		fmt.Print("Type Command or exit: ")
		scanner.Scan()
		input := scanner.Text()
		if input == "exit" {
			fmt.Println("Exiting...")
			break
		} else if input == "test" {
			test(ctx, projector)
		} else {
			fmt.Println(processCMD(ctx, projector, input))
			fmt.Println("")
		}

	}
}

func test(ctx context.Context, projector Projector) {
	fmt.Println("Testing Commands:")

	process := func(command string) {
		fmt.Println(command)
		fmt.Println(processCMD(ctx, projector, command))
		fmt.Println("")
	}

	commands := []string{
		"modelname ?",
		"ipv4_ip_address ?",
		"ipv4_default_gateway ?",
		"ipv4_dns_server1 ?",
		"ipv4_dns_server2 ?",
		"mac_address ?",
		"serialnum ?",
		"filter_status ?",
		"power_status ?",
		"warning ?",
		"error ?",
		"timer ?",
	}

	for _, command := range commands {
		process(command)
	}

}

func processCMD(ctx context.Context, projector Projector, input string) string {
	cmd := []byte(input + "\r\n")
	response, err := projector.SendCommand(ctx, projector.Address, cmd)
	if err != nil {
		return string(err.Error())
	} else {
		return response
	}
}

type Projector struct {
	poolInit sync.Once
	pool     *pooled.Pool
	Address  string
}

// GetHardwareInfo .
func GetHardwareInfo(address string) (structs.HardwareInfo, error) {
	var info structs.HardwareInfo

	work := func(conn pooled.Conn) error {
		conn.Log().Infof("Getting hardware info")

		// model name
		cmd := []byte("modelname ?\r\n")
		resp, err := writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.ModelName = strings.Trim(resp, "\"")

		// ip address
		cmd = []byte("ipv4_ip_address ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.NetworkInfo.IPAddress = strings.Trim(resp, "\"")

		// gateway
		cmd = []byte("ipv4_default_gateway ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.NetworkInfo.Gateway = strings.Trim(resp, "\"")

		// dns
		cmd = []byte("ipv4_dns_server1 ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.NetworkInfo.DNS = append(info.NetworkInfo.DNS, strings.Trim(resp, "\""))

		cmd = []byte("ipv4_dns_server2 ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.NetworkInfo.DNS = append(info.NetworkInfo.DNS, strings.Trim(resp, "\""))

		// mac address
		cmd = []byte("mac_address ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.NetworkInfo.MACAddress = strings.Trim(resp, "\"")

		// serial number
		cmd = []byte("serialnum ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.SerialNumber = strings.Trim(resp, "\"")

		// filter status
		cmd = []byte("filter_status ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.FilterStatus = strings.Trim(resp, "\"")

		// power status
		cmd = []byte("power_status ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		info.PowerStatus = strings.Trim(resp, "\"")

		// warnings
		cmd = []byte("warning ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		err = json.Unmarshal([]byte(resp), &info.WarningStatus)
		if err != nil {
			return err
		}

		// errors
		cmd = []byte("error ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		err = json.Unmarshal([]byte(resp), &info.ErrorStatus)
		if err != nil {
			return err
		}

		// timer info
		cmd = []byte("timer ?\r\n")
		resp, err = writeAndRead(conn, cmd, 3*time.Second)
		if err != nil {
			return err
		}

		err = json.Unmarshal([]byte(resp), &info.TimerInfo)
		if err != nil {
			return err
		}

		conn.Log().Infof("Hardware info %+v", info)

		return nil
	}

	err := pooled.Do(address, work)
	if err != nil {
		return info, err
	}

	return info, nil
}

func writeAndRead(conn pooled.Conn, cmd []byte, timeout time.Duration) (string, error) {
	conn.SetWriteDeadline(time.Now().Add(timeout))

	n, err := conn.Write(cmd)
	switch {
	case err != nil:
		return "", err
	case n != len(cmd):
		return "", fmt.Errorf("wrote %v/%v bytes of command 0x%x", n, len(cmd), cmd)
	}

	b, err := conn.ReadUntil('\n', timeout)
	if err != nil {
		return "", err
	}

	conn.Log().Debugf("Response from command: 0x%x", b)
	return strings.TrimSpace(string(b)), nil
}

func getConnection(key interface{}) (pooled.Conn, error) {
	address, ok := key.(string)
	if !ok {
		return nil, fmt.Errorf("key must be a string")
	}

	conn, err := net.DialTimeout("tcp", address+":53595", 10*time.Second)
	if err != nil {
		return nil, err
	}

	// read the NOKEY line
	pconn := pooled.Wrap(conn)
	b, err := pconn.ReadUntil('\n', 60*time.Second)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if strings.TrimSpace(string(b)) != "NOKEY" {
		conn.Close()
		return nil, fmt.Errorf("unexpected message when opening connection: %s", b)
	}
	return pconn, nil
}

// SendCommand sends the byte array to the desired address of the projector
func (p *Projector) SendCommand(ctx context.Context, addr string, cmd []byte) (string, error) {
	//using the Do method of sync.Once to make sure that the function is executed only once

	var resp []byte
	err := p.pool.Do(addr, func(conn pooled.Conn) error {
		conn.SetWriteDeadline(time.Now().Add(3 * time.Second))

		n, err := conn.Write(cmd)
		switch {
		case err != nil:
			return err
		case n != len(cmd):
			return fmt.Errorf("wrote %v/%v bytes of command 0x%x", n, len(cmd), cmd)
		}

		resp = make([]byte, 5)

		resp, err = conn.ReadUntil('\n', 3*time.Second)
		if err != nil {
			return err
		}

		conn.Log().Debugf("Response from command: 0x%x", resp)

		return nil
	})
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(resp)), nil
}
