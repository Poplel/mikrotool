package main
// sshWGAdder V1.01 made by Elan Fergusson
// date: 11/5/2025

import (
	"bytes"
	"fmt"
	"regexp"
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
	sshUser        = "admin"
	wgInterface    = "wireguard1"
	wgServerPort   = "443"
	clientDNS      = "10.10.10.1"
	defaultPeerKey = "private key from wireguard client"
	versionNumber  = "V1.01"
)


func main() {
	// By setting the appid here it forces fyne to actually save the fields in the right place
	myApp := app.NewWithID("com.wirestar.mikrotik-wg-tool")
	//set window
	myApp.SetIcon(resourceIconPng)
	myWindow := myApp.NewWindow("Mikrotik WireGuard Adding Tool")
	myWindow.Resize(fyne.NewSize(600, 500))

	prefs := myApp.Preferences()
	
	//set all the widgets/fields and fallback text
	//tik ip field
	mikrotikIPEntry := widget.NewEntry()
	mikrotikIPEntry.SetText(prefs.StringWithFallback("mikrotikIP", "23.92.1.5"))
	mikrotikIPEntry.OnChanged = func(s string) {
		prefs.SetString("mikrotikIP", s)
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
	//client private key
	clientPrivKeyEntry := widget.NewPasswordEntry()
	clientPrivKeyEntry.SetPlaceHolder(defaultPeerKey)
	clientPrivKeyEntry.SetText(prefs.String("clientPrivKey"))
	clientPrivKeyEntry.OnChanged = func(s string) {
		prefs.SetString("clientPrivKey", s)
	}
	//allowed address
	clientAllowedAddressEntry := widget.NewEntry()
	clientAllowedAddressEntry.SetText(prefs.StringWithFallback("allowedAddress", "10.10.10.200/32"))
	clientAllowedAddressEntry.OnChanged = func(s string) {
		prefs.SetString("allowedAddress", s)
	}

	// output widget
	outputBox := widget.NewLabel("Mikrotik WireGuard Generator - Made by Elan Fergusson " + versionNumber) 
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
		mikrotikIP := mikrotikIPEntry.Text
		sshPass := sshPassEntry.Text
		username := usernameEntry.Text
		clientPrivKey := clientPrivKeyEntry.Text
		clientAllowedAddress := clientAllowedAddressEntry.Text

		//set variables based on constants
		sshHost := mikrotikIP
		wgServerEndpoint := mikrotikIP

		// input validation
		if mikrotikIP == "" || sshPass == "" || username == "" || clientPrivKey == "" || clientAllowedAddress == "" {
			dialog.ShowError(fmt.Errorf("all fields are mandatory"), myWindow)
			return
		}

		// validate ip
		if !ipRegex.MatchString(mikrotikIP) {
			dialog.ShowError(fmt.Errorf("mikroTik IP must be a valid IP address (e.g., 23.92.1.5)"), myWindow)
			return
		}

		// validate username
		if !usernameRegex.MatchString(username) {
			dialog.ShowError(fmt.Errorf("username must be 1-25 characters and only contain letters, numbers, '-', '_', or '.'"), myWindow)
			return
		}

		// validate allowed address
		if !cidrRegex.MatchString(clientAllowedAddress) {
			dialog.ShowError(fmt.Errorf("client allowed address must be a valid ip with CIDR block notation (e.g., 10.10.10.200/32)"), myWindow)
			return
		}

		// validate priv key
		if clientPrivKey == defaultPeerKey {
			dialog.ShowError(fmt.Errorf("Please replace the placeholder text with a real client private key"), myWindow)
			return
		}

		// Process key
		appendLog("Validating client private key...")
		clientPrivKey = strings.TrimSpace(clientPrivKey) // trim whitespace
		privKey, err := wgtypes.ParseKey(clientPrivKey)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid client private key: %w", err), myWindow)
			appendLog(fmt.Sprintf("ERROR: Invalid client private key: %s", err.Error()))
			return
		}
		clientPubKey := privKey.PublicKey().String()
		appendLog(fmt.Sprintf("Calculated client public key: %s", clientPubKey))

		// make command to add peer from field input
		addPeerCmd := fmt.Sprintf(
			"/interface wireguard peers add interface=%s public-key=\"%s\" allowed-address=%s name=\"%s\"",
			wgInterface,
			clientPubKey,
			clientAllowedAddress,
			username,
		)

		appendLog(fmt.Sprintf("Connecting to %s as user '%s'...", sshHost, sshUser))
		//ssh to tik
		sshOut, err := runSSHCommand(sshHost, sshUser, sshPass, addPeerCmd)
		//check errors
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "already have peer") {
				appendLog("Peer with this public key already exists. Skipping add...")
			} else if strings.Contains(errStr, "Process exited with status 1 (stderr: )") {
				appendLog("Peer add failed (Exit Status 1). This likely means the peer username or IP already exists.")
			} else {
				dialog.ShowError(fmt.Errorf("failed to add peer: %w", err), myWindow)
				appendLog(fmt.Sprintf("ERROR adding peer: %s", errStr))
				return
			}
		} else {
			appendLog(fmt.Sprintf("Peer added successfully. MikroTik response: %s", sshOut))
		}

		//ssh to get public key from tik
		getServerKeyCmd := fmt.Sprintf(":put [/interface wireguard get [find name=\"%s\"] value-name=public-key]", wgInterface) // hardcoded
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
			clientAllowedAddress,
			clientDNS,
			serverPubKey,
			wgServerEndpoint,
			wgServerPort,
		)

		outputBox.SetText(finalConfig)
		copyButton.Show() // show copy button 
		dialog.ShowInformation("Success!", "Client config has been generated in the output box.", myWindow) //success messgage
	})

	///build window layout
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "MikroTik IP", Widget: mikrotikIPEntry},
			{Text: "SSH Password", Widget: sshPassEntry},
			{Text: "Username", Widget: usernameEntry},
			{Text: "Client Private Key", Widget: clientPrivKeyEntry},
			{Text: "Client Allowed Address", Widget: clientAllowedAddressEntry},
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
//ssh function
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
