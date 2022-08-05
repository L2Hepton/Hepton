// Copyright 2014 The go-ethereum Authors
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

package core

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go
//go:generate gencodec -type Init -field-override initMarshaling -out gen_genesis_init.go
//go:generate gencodec -type LockedAccount -field-override lockedAccountMarshaling -out gen_genesis_locked_account.go
//go:generate gencodec -type ValidatorInfo -field-override validatorInfoMarshaling -out gen_genesis_validator_info.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`
	Validators []ValidatorInfo     `json:"validators" gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
	BaseFee    *big.Int    `json:"baseFeePerGas"`
}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance"            gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	Init       *Init                       `json:"init,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}

// InitArgs represents the args of system contracts inital args
type Init struct {
	Admin           common.Address  `json:"admin,omitempty"`
	FirstLockPeriod *big.Int        `json:"firstLockPeriod,omitempty"`
	ReleasePeriod   *big.Int        `json:"releasePeriod,omitempty"`
	ReleaseCnt      *big.Int        `json:"releaseCnt,omitempty"`
	RuEpoch         *big.Int        `json:"ruEpoch,omitempty"`
	PeriodTime      *big.Int        `json:"periodTime,omitempty"`
	LockedAccounts  []LockedAccount `json:"lockedAccounts,omitempty"`
}

// LockedAccount represents the info of the locked account
type LockedAccount struct {
	UserAddress  common.Address `json:"userAddress,omitempty"`
	TypeId       *big.Int       `json:"typeId,omitempty"`
	LockedAmount *big.Int       `json:"lockedAmount,omitempty"`
	LockedTime   *big.Int       `json:"lockedTime,omitempty"`
	PeriodAmount *big.Int       `json:"periodAmount,omitempty"`
}

// ValidatorInfo represents the info of inital validators
type ValidatorInfo struct {
	Address          common.Address `json:"address"         gencodec:"required"`
	Manager          common.Address `json:"manager"         gencodec:"required"`
	Rate             *big.Int       `json:"rate,omitempty"`
	Stake            *big.Int       `json:"stake,omitempty"`
	AcceptDelegation bool           `json:"acceptDelegation,omitempty"`
}

// makeValidator creates ValidatorInfo
func makeValidator(address, manager, rate, stake string, acceptDelegation bool) ValidatorInfo {
	rateNum, ok := new(big.Int).SetString(rate, 10)
	if !ok {
		panic("Failed to make validator info due to invalid rate")
	}
	stakeNum, ok := new(big.Int).SetString(stake, 10)
	if !ok {
		panic("Failed to make validator info due to invalid stake")
	}

	return ValidatorInfo{
		Address:          common.HexToAddress(address),
		Manager:          common.HexToAddress(manager),
		Rate:             rateNum,
		Stake:            stakeNum,
		AcceptDelegation: acceptDelegation,
	}
}

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce      math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Difficulty *math.HexOrDecimal256
	BaseFee    *math.HexOrDecimal256
	Alloc      map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

type initMarshaling struct {
	FirstLockPeriod *math.HexOrDecimal256
	ReleasePeriod   *math.HexOrDecimal256
	ReleaseCnt      *math.HexOrDecimal256
	RuEpoch         *math.HexOrDecimal256
	PeriodTime      *math.HexOrDecimal256
}

type lockedAccountMarshaling struct {
	TypeId       *math.HexOrDecimal256
	LockedAmount *math.HexOrDecimal256
	LockedTime   *math.HexOrDecimal256
	PeriodAmount *math.HexOrDecimal256
}

type validatorInfoMarshaling struct {
	Rate  *math.HexOrDecimal256
	Stake *math.HexOrDecimal256
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database contains incompatible genesis (have %x, new %x)", e.Stored, e.New)
}

// SetupGenesisBlock writes or updates the genesis block in db.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     db has no genesis |  main-net default  |  genesis
//     db has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	return SetupGenesisBlockWithOverride(db, genesis, nil)
}

func SetupGenesisBlockWithOverride(db ethdb.Database, genesis *Genesis, overrideArrowGlacier *big.Int) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, common.Hash{}, err
		}
		return genesis.Config, block.Hash(), nil
	}
	// We have the genesis block in database(perhaps in ancient database)
	// but the corresponding state is missing.
	header := rawdb.ReadHeader(db, stored, 0)
	if _, err := state.New(header.Root, state.NewDatabaseWithConfig(db, nil), nil); err != nil {
		if genesis == nil {
			genesis = DefaultGenesisBlock()
		}
		// Ensure the stored genesis matches with the given one.
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
		block, err := genesis.Commit(db)
		if err != nil {
			return genesis.Config, hash, err
		}
		return genesis.Config, block.Hash(), nil
	}
	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}
	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)
	if overrideArrowGlacier != nil {
		newcfg.ArrowGlacierBlock = overrideArrowGlacier
	}
	if err := newcfg.CheckConfigForkOrder(); err != nil {
		return newcfg, common.Hash{}, err
	}
	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != params.MainnetGenesisHash {
		return storedcfg, stored, nil
	}
	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	// Check whether consensus config of Hapten is changed
	if (storedcfg.Hapten != nil || newcfg.Hapten != nil) && (storedcfg.Hapten == nil ||
		newcfg.Hapten == nil || *storedcfg.Hapten != *newcfg.Hapten) {
		return nil, common.Hash{}, errors.New("HaptenConfig is not compatiable with stored")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	default:
		return params.AllHaptenProtocolChanges
	}
}

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = rawdb.NewMemoryDatabase()
	}
	statedb, err := state.New(common.Hash{}, state.NewDatabase(db), nil)
	if err != nil {
		panic(err)
	}
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}
	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       g.Timestamp,
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		BaseFee:    g.BaseFee,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}
	if g.Config != nil && g.Config.IsLondon(common.Big0) {
		if g.BaseFee != nil {
			head.BaseFee = g.BaseFee
		} else {
			head.BaseFee = new(big.Int).SetUint64(params.InitialBaseFee)
		}
	}

	// Handle the Hapten related
	if g.Config.Hapten != nil {
		// init system contract
		gInit := &genesisInit{statedb, head, g}
		for name, initSystemContract := range map[string]func() error{
			"Staking":       gInit.initStaking,
			"CommunityPool": gInit.initCommunityPool,
			"BonusPool":     gInit.initBonusPool,
			"GenesisLock":   gInit.initGenesisLock,
		} {
			if err = initSystemContract(); err != nil {
				log.Crit("Failed to init system contract", "contract", name, "err", err)
			}
		}
		// Set validoter info
		if head.Extra, err = gInit.initValidators(); err != nil {
			log.Crit("Failed to init Validators", "err", err)
		}
	}

	// Update root after execution
	head.Root = statedb.IntermediateRoot(false)

	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(head.Root, true, nil)

	return types.NewBlock(head, nil, nil, nil, trie.NewStackTrie(nil))
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, errors.New("can't commit genesis block with number > 0")
	}
	config := g.Config
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
	if err := config.CheckConfigForkOrder(); err != nil {
		return nil, err
	}
	if config.Clique != nil && len(block.Extra()) == 0 {
		return nil, errors.New("can't start clique chain without signers")
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), block.Difficulty())
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadFastBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to db, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{
		Alloc:   GenesisAlloc{addr: {Balance: balance}},
		BaseFee: big.NewInt(params.InitialBaseFee),
	}
	return g.MustCommit(db)
}

// DefaultGenesisBlock returns the Ethereum main net genesis block.
func DefaultGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.MainnetChainConfig,
		Nonce:      0,
		Timestamp:  0x62b02908,
		ExtraData:  hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   0x280de80,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(mainnetAllocData),
		Validators: []ValidatorInfo{
			makeValidator("0xC8F660906B413027000e583E00a5ea0008370755", "0x07Ff2F6e4bDA2a899F71735C7071F061D0bb3647", "20", "350000000000000000000", true),
		},
	}
}

func DefaultTestnetGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.TestnetChainConfig,
		Timestamp:  0x62b02908,
		ExtraData:  hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   0x05f5e100,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(testnetAllocData),
		Mixhash:    common.Hash{},
		Validators: []ValidatorInfo{
			makeValidator("0x01ec6c288cA26a0D896204034484D1d7a3E3230D", "0x17Bc6809697dA4022cd24C75ee67d65EBf30d89a", "0", "1000000", false),
			makeValidator("0xcc09ADD873B0fb23cc8572F64739a503F1000493", "0x17Bc6809697dA4022cd24C75ee67d65EBf30d89a", "0", "1000000", true),
			makeValidator("0x283E674870C505A51925cC3fC7b5dA7EF03e4C3A", "0x17Bc6809697dA4022cd24C75ee67d65EBf30d89a", "50", "1000000", true),
			makeValidator("0x9c0F9CD45b685448254311181d637C91898cFFB0", "0x17Bc6809697dA4022cd24C75ee67d65EBf30d89a", "100", "1000000", true),
			makeValidator("0x100694D6cCb57ACF81a90a46A662B274254F69BC", "0x17Bc6809697dA4022cd24C75ee67d65EBf30d89a", "100", "1000000", false),
		},
	}
}

// DefaultRopstenGenesisBlock returns the Ropsten network genesis block.
func DefaultRopstenGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RopstenChainConfig,
		Nonce:      66,
		ExtraData:  hexutil.MustDecode("0x3535353535353535353535353535353535353535353535353535353535353535"),
		GasLimit:   16777216,
		Difficulty: big.NewInt(1048576),
		Alloc:      decodePrealloc(ropstenAllocData),
	}
}

// BasicHaptenGenesisBlock returns a genesis containing basic allocation for Chais engine,
func BasicHaptenGenesisBlock(config *params.ChainConfig, initialValidators []common.Address, faucet common.Address) *Genesis {
	extraVanity := 32
	extraData := make([]byte, extraVanity+common.AddressLength*len(initialValidators)+65)
	for i, validator := range initialValidators {
		copy(extraData[extraVanity+i*common.AddressLength:], validator[:])
	}
	alloc := decodePrealloc(basicAllocForHapten)
	if (faucet != common.Address{}) {
		// 100M
		b, _ := new(big.Int).SetString("100000000000000000000000000", 10)
		alloc[faucet] = GenesisAccount{Balance: b}
	}
	return &Genesis{
		Config:     config,
		ExtraData:  extraData,
		GasLimit:   0x280de80,
		Difficulty: big.NewInt(2),
		Alloc:      alloc,
	}
}

// DeveloperGenesisBlock returns the 'geth --dev' genesis block.
func DeveloperGenesisBlock(period uint64, gasLimit uint64, faucet common.Address) *Genesis {
	// Override the default period to the user requested one
	config := *params.AllCliqueProtocolChanges
	config.Clique = &params.CliqueConfig{
		Period: period,
		Epoch:  config.Clique.Epoch,
	}

	// Assemble and return the genesis with the precompiles and faucet pre-funded
	return &Genesis{
		Config:     &config,
		ExtraData:  append(append(make([]byte, 32), faucet[:]...), make([]byte, crypto.SignatureLength)...),
		GasLimit:   gasLimit,
		BaseFee:    big.NewInt(params.InitialBaseFee),
		Difficulty: big.NewInt(1),
		Alloc: map[common.Address]GenesisAccount{
			common.BytesToAddress([]byte{1}): {Balance: big.NewInt(1)}, // ECRecover
			common.BytesToAddress([]byte{2}): {Balance: big.NewInt(1)}, // SHA256
			common.BytesToAddress([]byte{3}): {Balance: big.NewInt(1)}, // RIPEMD
			common.BytesToAddress([]byte{4}): {Balance: big.NewInt(1)}, // Identity
			common.BytesToAddress([]byte{5}): {Balance: big.NewInt(1)}, // ModExp
			common.BytesToAddress([]byte{6}): {Balance: big.NewInt(1)}, // ECAdd
			common.BytesToAddress([]byte{7}): {Balance: big.NewInt(1)}, // ECScalarMul
			common.BytesToAddress([]byte{8}): {Balance: big.NewInt(1)}, // ECPairing
			common.BytesToAddress([]byte{9}): {Balance: big.NewInt(1)}, // BLAKE2b
			faucet:                           {Balance: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(9))},
		},
	}
}

func decodePrealloc(data string) GenesisAlloc {
	type locked struct {
		UserAddress  *big.Int
		TypeId       *big.Int
		LockedAmount *big.Int
		LockedTime   *big.Int
		PeriodAmount *big.Int
	}

	type initArgs struct {
		Admin           *big.Int
		FirstLockPeriod *big.Int
		ReleasePeriod   *big.Int
		ReleaseCnt      *big.Int
		RuEpoch         *big.Int
		PeriodTime      *big.Int
		LockedAccounts  []locked
	}

	var p []struct {
		Addr    *big.Int
		Balance *big.Int
		Code    []byte
		Init    *initArgs
	}

	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		var init *Init
		if account.Init != nil {
			init = &Init{
				Admin:           common.BigToAddress(account.Init.Admin),
				FirstLockPeriod: account.Init.FirstLockPeriod,
				ReleasePeriod:   account.Init.ReleasePeriod,
				ReleaseCnt:      account.Init.ReleaseCnt,
				RuEpoch:         account.Init.RuEpoch,
				PeriodTime:      account.Init.PeriodTime,
			}
			if len(account.Init.LockedAccounts) > 0 {
				init.LockedAccounts = make([]LockedAccount, 0, len(account.Init.LockedAccounts))
				for _, locked := range account.Init.LockedAccounts {
					init.LockedAccounts = append(init.LockedAccounts,
						LockedAccount{
							UserAddress:  common.BigToAddress(locked.UserAddress),
							TypeId:       locked.TypeId,
							LockedAmount: locked.LockedAmount,
							LockedTime:   locked.LockedTime,
							PeriodAmount: locked.PeriodAmount,
						})
				}
			}
		}
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance, Code: account.Code, Init: init}
	}
	return ga
}
