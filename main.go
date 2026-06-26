package main

// sshWGAdder V1.11 made by Elan Fergusson
// date: 6/25/2026

import (
	"bytes"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/crypto/ssh"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

var (
	// IP regex for tik ip
	ipRegex = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	// CIDR regex for allowed ip
	cidrRegex = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}/\d{1,2}$`)
	// Username regex 1-25 chars, letters, numbers, and -_.
	usernameRegex = regexp.MustCompile(`^[a-zA-Z0-9-_.]{1,25}$`)
)

// Default config
const (
	appTitle      = "SSH WireGuard Tool for RouterOS"
	versionNumber = "V1.11"
)

func main() {
	// By setting the appid here it forces fyne to actually save the fields in the right place
	myApp := app.NewWithID("com.wirestar.mikrotik-wg-tool")
	//set window
	myApp.SetIcon(resourceIconPng)
	myWindow := myApp.NewWindow(appTitle)
	myWindow.Resize(fyne.NewSize(600, 500))

	prefs := myApp.Preferences()

	//set all the widgets/fields and fallback text
	//tik ip field
	mikrotikIPEntry := widget.NewEntry()
	mikrotikIPEntry.SetText(prefs.StringWithFallback("mikrotikIP", "23.92.1.5"))
	mikrotikIPEntry.OnChanged = func(s string) {
		prefs.SetString("mikrotikIP", s)
	}
	//ssh username
	sshUserEntry := widget.NewEntry()
	sshUserEntry.SetText(prefs.String("sshUser"))
	sshUserEntry.OnChanged = func(s string) {
		prefs.SetString("sshUser", s)
	}
	//ssh password
	sshPassEntry := widget.NewPasswordEntry()
	sshPassEntry.SetText(prefs.StringWithFallback("sshPass", "password"))
	sshPassEntry.OnChanged = func(s string) {
		prefs.SetString("sshPass", s)
	}

	//peer username
	usernameEntry := widget.NewEntry()
	usernameEntry.SetText(prefs.StringWithFallback("username", "new-user"))
	usernameEntry.OnChanged = func(s string) {
		prefs.SetString("username", s)
	}
	//allowed address
	clientAllowedAddressEntry := widget.NewEntry()
	clientAllowedAddressEntry.SetText(prefs.StringWithFallback("allowedAddress", "10.10.10.200/32"))
	clientAllowedAddressEntry.OnChanged = func(s string) {
		prefs.SetString("allowedAddress", s)
	}
	//wireguard server port
	wgServerPortEntry := widget.NewEntry()
	wgServerPortEntry.SetText(prefs.StringWithFallback("wgServerPort", "443"))
	wgServerPortEntry.OnChanged = func(s string) {
		prefs.SetString("wgServerPort", s)
	}

	// output widget
	outputBox := widget.NewLabel(appTitle + " - Made by Elan Fergusson " + versionNumber)
	outputBox.Wrapping = fyne.TextWrapWord
	outputScroll := container.NewScroll(outputBox)

	// append to output widget
	appendLog := func(msg string) {
		currentText := outputBox.Text
		outputBox.SetText(currentText + msg + "\n")
	}

	//copy config button
	copyButton := widget.NewButton("Copy Config", func() {
		myWindow.Clipboard().SetContent(outputBox.Text)
	})
	//default hidden til the config is made
	copyButton.Hide()

	// generate button
	goButton := widget.NewButton("Add Peer and Generate Config", func() {
		outputBox.SetText("") // clear output box
		copyButton.Hide()     // Hide copy button on new run

		// get fields
		mikrotikIP := strings.TrimSpace(mikrotikIPEntry.Text)
		sshUser := strings.TrimSpace(sshUserEntry.Text)
		sshPass := sshPassEntry.Text
		peerName := strings.TrimSpace(usernameEntry.Text)
		peerAddress := strings.TrimSpace(clientAllowedAddressEntry.Text)
		wgServerPort := strings.TrimSpace(wgServerPortEntry.Text)

		//set variables based on constants
		sshHost := mikrotikIP
		wgServerEndpoint := mikrotikIP

		// input validation
		if mikrotikIP == "" || sshUser == "" || sshPass == "" || peerName == "" || peerAddress == "" || wgServerPort == "" {
			dialog.ShowError(fmt.Errorf("all fields are mandatory"), myWindow)
			return
		}

		// validate ip
		if !ipRegex.MatchString(mikrotikIP) {
			dialog.ShowError(fmt.Errorf("mikroTik IP must be a valid IP address (e.g., 23.92.1.5)"), myWindow)
			return
		}

		// validate ssh username
		if !usernameRegex.MatchString(sshUser) {
			dialog.ShowError(fmt.Errorf("ssh username must be 1-25 characters and only contain letters, numbers, '-', '_', or '.'"), myWindow)
			return
		}

		// validate username
		if !usernameRegex.MatchString(peerName) {
			dialog.ShowError(fmt.Errorf("peer name must be 1-25 characters and only contain letters, numbers, '-', '_', or '.'"), myWindow)
			return
		}

		// validate peer address
		if !cidrRegex.MatchString(peerAddress) {
			dialog.ShowError(fmt.Errorf("peer address must be a valid ip with CIDR block notation (e.g., 10.10.10.200/32)"), myWindow)
			return
		}

		wgServerPortNum, err := strconv.Atoi(wgServerPort)
		if err != nil {
			dialog.ShowError(fmt.Errorf("WireGuard port must be a number from 1 to 65535"), myWindow)
			return
		}
		if wgServerPortNum < 1 || wgServerPortNum > 65535 {
			dialog.ShowError(fmt.Errorf("WireGuard port must be between 1 and 65535"), myWindow)
			return
		}
		wgServerPort = strconv.Itoa(wgServerPortNum)

		addPeerForInterface := func(wgInterface string) {
			// Generate key pair
			appendLog("Generating client key pair...")
			privKey, err := wgtypes.GeneratePrivateKey()
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to generate client private key: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR: Failed to generate client private key: %s", err.Error()))
				return
			}
			clientPrivKey := privKey.String()
			clientPubKey := privKey.PublicKey().String()
			appendLog(fmt.Sprintf("Generated client public key: %s", clientPubKey))

			// get the WireGuard interface address so DNS and subnet match the router
			getWGAddressCmd := fmt.Sprintf(":foreach addrId in=[/ip address find interface=\"%s\" disabled=no] do={:put [/ip address get $addrId address]}", wgInterface)
			appendLog(fmt.Sprintf("Fetching %s address...", wgInterface))
			wgAddressOut, err := runSSHCommand(sshHost, sshUser, sshPass, getWGAddressCmd)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to get WireGuard address: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR fetching WireGuard address: %s", err.Error()))
				return
			}

			dnsServerIP, wgSubnetCIDR, wgSubnet, err := parseWireGuardAddress(wgAddressOut)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to parse WireGuard address: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR parsing WireGuard address: %s", err.Error()))
				return
			}
			appendLog(fmt.Sprintf("Detected WireGuard subnet %s with DNS %s", wgSubnetCIDR, dnsServerIP))

			peerIP, _, err := net.ParseCIDR(peerAddress)
			if err != nil || peerIP.To4() == nil {
				dialog.ShowError(fmt.Errorf("peer address must be a valid IPv4 CIDR address"), myWindow)
				appendLog(fmt.Sprintf("ERROR: Invalid peer address: %s", peerAddress))
				return
			}
			if !wgSubnet.Contains(peerIP) {
				dialog.ShowError(fmt.Errorf("peer address must be inside WireGuard subnet %s", wgSubnetCIDR), myWindow)
				appendLog(fmt.Sprintf("ERROR: %s is outside WireGuard subnet %s", peerAddress, wgSubnetCIDR))
				return
			}

			// make command to add peer from field input
			addPeerCmd := fmt.Sprintf(
				"/interface wireguard peers add interface=%s public-key=\"%s\" allowed-address=%s name=\"%s\"",
				wgInterface,
				clientPubKey,
				peerAddress,
				peerName,
			)
			removePeerCmd := fmt.Sprintf("/interface wireguard peers remove [find name=\"%s\"]", peerName)

			appendLog(fmt.Sprintf("Removing any existing peer named '%s'...", peerName))
			removeOut, err := runSSHCommand(sshHost, sshUser, sshPass, removePeerCmd)
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to remove existing peer: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR removing existing peer: %s", err.Error()))
				return
			}
			if strings.TrimSpace(removeOut) == "" {
				appendLog("Existing peer removed if present.")
			} else {
				appendLog(fmt.Sprintf("Remove peer response: %s", strings.TrimSpace(removeOut)))
			}

			//ssh to tik
			sshOut, err := runSSHCommand(sshHost, sshUser, sshPass, addPeerCmd)
			//check errors
			if err != nil {
				errStr := err.Error()
				if strings.Contains(errStr, "already have peer") {
					appendLog("Peer with this public key already exists. Skipping add...")
				} else if strings.Contains(errStr, "Process exited with status 1 (stderr: )") {
					appendLog("Peer add failed (Exit Status 1). This likely means the peer name or IP already exists.")
				} else {
					dialog.ShowError(fmt.Errorf("failed to add peer: %w", err), myWindow)
					appendLog(fmt.Sprintf("ERROR adding peer: %s", errStr))
					return
				}
			} else {
				appendLog(fmt.Sprintf("Peer added successfully. MikroTik response: %s", sshOut))
			}

			//ssh to get public key from tik
			getServerKeyCmd := fmt.Sprintf(":put [/interface wireguard get [find name=\"%s\"] value-name=public-key]", wgInterface)
			appendLog("Fetching server public key...")
			serverPubKey, err := runSSHCommand(sshHost, sshUser, sshPass, getServerKeyCmd)
			//error handling
			if err != nil {
				dialog.ShowError(fmt.Errorf("failed to get server public key: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR fetching server key: %s", err.Error()))
				return
			}
			serverPubKey = strings.TrimSpace(serverPubKey) //trim whitespace
			if serverPubKey == "" {
				dialog.ShowError(fmt.Errorf("got blank public key from server. Check interface name '%s'", wgInterface), myWindow)
				appendLog(fmt.Sprintf("ERROR: Server returned a blank public key. Is '%s' the correct interface name?", wgInterface))
				return
			}
			appendLog(fmt.Sprintf("Got server public key: %s", serverPubKey))

			//put together config output
			appendLog("Generating client configuration...")
			finalConfig := fmt.Sprintf(
				`[Interface]
PrivateKey = %s
Address = %s
DNS = %s
MTU = 1420

[Peer]
PublicKey = %s
AllowedIPs = 0.0.0.0/0
Endpoint = %s:%s
`,
				clientPrivKey,
				peerAddress,
				dnsServerIP,
				serverPubKey,
				wgServerEndpoint,
				wgServerPort,
			)

			outputBox.SetText(finalConfig)
			copyButton.Show()                                                                                   // show copy button
			dialog.ShowInformation("Success!", "Client config has been generated in the output box.", myWindow) //success messgage
		}

		appendLog(fmt.Sprintf("Connecting to %s as user '%s'...", sshHost, sshUser))
		appendLog("Detecting WireGuard interfaces...")
		getWGInterfacesCmd := ":foreach ifId in=[/interface wireguard find] do={:put [/interface wireguard get $ifId name]}"
		wgInterfacesOut, err := runSSHCommand(sshHost, sshUser, sshPass, getWGInterfacesCmd)
		if err != nil {
			dialog.ShowError(fmt.Errorf("failed to detect WireGuard interfaces: %w", err), myWindow)
			appendLog(fmt.Sprintf("ERROR detecting WireGuard interfaces: %s", err.Error()))
			return
		}
		wgInterfaces := parseRouterOSLines(wgInterfacesOut)
		if len(wgInterfaces) == 0 {
			dialog.ShowError(fmt.Errorf("no WireGuard interfaces found on router"), myWindow)
			appendLog("ERROR: No WireGuard interfaces found on router.")
			return
		}
		if len(wgInterfaces) == 1 {
			appendLog(fmt.Sprintf("Using WireGuard interface '%s'.", wgInterfaces[0]))
			addPeerForInterface(wgInterfaces[0])
			return
		}

		appendLog(fmt.Sprintf("Found %d WireGuard interfaces.", len(wgInterfaces)))
		interfaceSelect := widget.NewSelect(wgInterfaces, nil)
		interfaceSelect.SetSelected(wgInterfaces[0])
		dialog.ShowCustomConfirm(
			"Choose WireGuard Interface",
			"Use Interface",
			"Cancel",
			container.NewVBox(
				widget.NewLabel("Select the WireGuard interface to use."),
				interfaceSelect,
			),
			func(useInterface bool) {
				if !useInterface {
					appendLog("Cancelled WireGuard interface selection.")
					return
				}
				if interfaceSelect.Selected == "" {
					dialog.ShowError(fmt.Errorf("select a WireGuard interface"), myWindow)
					return
				}
				appendLog(fmt.Sprintf("Using WireGuard interface '%s'.", interfaceSelect.Selected))
				addPeerForInterface(interfaceSelect.Selected)
			},
			myWindow,
		)
	})

	///build window layout
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "MikroTik IP", Widget: mikrotikIPEntry},
			{Text: "SSH Username", Widget: sshUserEntry},
			{Text: "SSH Password", Widget: sshPassEntry},
			{Text: "Peer Name", Widget: usernameEntry},
			{Text: "Peer Address", Widget: clientAllowedAddressEntry},
			{Text: "WireGuard Port", Widget: wgServerPortEntry},
		},
	}

	content := container.NewBorder(
		container.NewVBox(widget.NewLabel("MikroTik & Client Details"), form),
		container.NewVBox(
			goButton,
			copyButton,
		),
		nil,
		nil,
		outputScroll,
	)

	myWindow.SetContent(content)
	myWindow.ShowAndRun()
}

func parseRouterOSLines(output string) []string {
	var lines []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || seen[line] {
			continue
		}
		seen[line] = true
		lines = append(lines, line)
	}

	return lines
}

func parseWireGuardAddress(output string) (string, string, *net.IPNet, error) {
	for _, field := range strings.Fields(output) {
		ip, subnet, err := net.ParseCIDR(field)
		if err == nil && ip.To4() != nil {
			return ip.String(), subnet.String(), subnet, nil
		}
	}

	return "", "", nil, fmt.Errorf("no valid IPv4 CIDR address found in output: %q", strings.TrimSpace(output))
}

// ssh function
func runSSHCommand(host, user, pass, cmd string) (string, error) {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // skip host key verification because i do not care about mitm
		Timeout:         10 * time.Second,
	}

	sshHost := host
	if !strings.Contains(sshHost, ":") {
		sshHost = sshHost + ":22"
	}
	//check tcp dial
	client, err := ssh.Dial("tcp", sshHost, config)
	if err != nil {
		return "", fmt.Errorf("failed to dial: %w", err)
	}
	defer client.Close()
	//create session
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("failed to run command: %w (stderr: %s)", err, stderr.String())
	}

	// check for failure in stderr
	if strings.Contains(stderr.String(), "failure") {
		return stdout.String(), fmt.Errorf("command produced error: %s", stderr.String())
	}

	return stdout.String(), nil //return
}
