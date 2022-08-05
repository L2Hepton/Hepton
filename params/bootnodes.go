// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package params

import "github.com/ethereum/go-ethereum/common"

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{
	// Ethereum Foundation Go Bootnodes
	"",
	"",
	"",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
var TestnetBootnodes = []string{
	"enode://4f5af83d4c16ae2ceae24b14847d158495b62ab9199a2071d2520d42306645aab53cc7c72dfd00adab29ce4e63972a3a61b1d2847fdf153ca24796242b85384a@216.107.21.39:30404",
	"enode://0c6aa6bc089414dc815fb0861cbf01ba8f3d6792bfd9bf907ec22c0a3e42c537a152e237e695a3f257d25e4efa616e9f521f79fd4f79dafcd02ce6a60f9dc645@207.199.137.98:30404",
	"enode://350260f93ecd9da18f400131242066b647f85589c10828cef60f9ac95121a2b971bfc379e6af8a95236a5ed2029414ff48375800cb922540308e9b645d77dde3@207.199.156.4:30404",
}

var V5Bootnodes = []string{}

// KnownDNSNetwork returns the address of a public DNS-based node list for the given
// genesis hash and protocol. See https://github.com/ethereum/discv4-dns-lists for more
// information.
func KnownDNSNetwork(genesis common.Hash, protocol string) string {
	return ""
}
