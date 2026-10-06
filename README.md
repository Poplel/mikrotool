# Mikrotool v2.2

Mikrotool is a Go/Fyne desktop utility for keeping a list of MikroTik router sites, opening them in WinBox, and creating transient WireGuard peer sessions through RouterOS SSH.

License
Copyright 2026 Elan Fergusson. Mikrotool is free and open-source software licensed under the GNU General Public License version 3 only (GPL-3.0-only). Individuals and businesses may use and modify it. Anyone who distributes Mikrotool or a modified version must preserve the GPL terms and make the corresponding source available.

What it does
Stores IP address, site ID, and site name records in a local CSV.
Imports Mikrotool CSV files and WinBox 3 .wbx managed-router lists.
Adds or updates a site from a mikrotool: link opened in a browser, and can immediately open WinBox or connect WireGuard to it.
Creates transient WireGuard connections via SSH.
Opens WinBox directly to avoid needing a seperate list of sites and IP addresses.
