package db

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/crypto/sha3"
)

func get_hash(s string) common.Hash {
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(s))
	return common.Hash(hash.Sum(nil))
}

var (
	// Azimuth Events
	SPAWNED                  = get_hash("Spawned(uint32,uint32)")
	ACTIVATED                = get_hash("Activated(uint32)")
	OWNER_CHANGED            = get_hash("OwnerChanged(uint32,address)")
	CHANGED_SPAWN_PROXY      = get_hash("ChangedSpawnProxy(uint32,address)")
	CHANGED_TRANSFER_PROXY   = get_hash("ChangedTransferProxy(uint32,address)")
	CHANGED_MANAGEMENT_PROXY = get_hash("ChangedManagementProxy(uint32,address)")
	CHANGED_VOTING_PROXY     = get_hash("ChangedVotingProxy(uint32,address)")
	ESCAPE_REQUESTED         = get_hash("EscapeRequested(uint32,uint32)")
	ESCAPE_CANCELED          = get_hash("EscapeCanceled(uint32,uint32)")
	ESCAPE_ACCEPTED          = get_hash("EscapeAccepted(uint32,uint32)")
	LOST_SPONSOR             = get_hash("LostSponsor(uint32,uint32)")
	BROKE_CONTINUITY         = get_hash("BrokeContinuity(uint32,uint32)")
	CHANGED_KEYS             = get_hash("ChangedKeys(uint32,bytes32,bytes32,uint32,uint32)")
	CHANGED_DNS              = get_hash("ChangedDns(string,string,string)")

	// Ecliptic Events
	// APPROVAL_FOR_ALL         = get_hash("ApprovalForAll(address,address,bool)")
	// OWNERSHIP_TRANSFERRED    = get_hash("OwnershipTransferred(address,address)")

	// Naive Events (TODO)
)

var EVENT_NAMES = map[common.Hash]string{}

func init() {
	EVENT_NAMES[OWNER_CHANGED] = "OwnerChanged"
	EVENT_NAMES[ACTIVATED] = "Activated"
	EVENT_NAMES[SPAWNED] = "Spawned"
	EVENT_NAMES[ESCAPE_REQUESTED] = "EscapeRequested"
	EVENT_NAMES[ESCAPE_CANCELED] = "EscapeCanceled"
	EVENT_NAMES[ESCAPE_ACCEPTED] = "EscapeAccepted"
	EVENT_NAMES[LOST_SPONSOR] = "LostSponsor"
	EVENT_NAMES[CHANGED_KEYS] = "ChangedKeys"
	EVENT_NAMES[BROKE_CONTINUITY] = "BrokeContinuity"
	EVENT_NAMES[CHANGED_SPAWN_PROXY] = "ChangedSpawnProxy"
	EVENT_NAMES[CHANGED_TRANSFER_PROXY] = "ChangedTransferProxy"
	EVENT_NAMES[CHANGED_MANAGEMENT_PROXY] = "ChangedManagementProxy"
	EVENT_NAMES[CHANGED_VOTING_PROXY] = "ChangedVotingProxy"
	EVENT_NAMES[CHANGED_DNS] = "ChangedDns"

	// EVENT_NAMES[APPROVAL_FOR_ALL] = "ApprovalForAll"
	// EVENT_NAMES[OWNERSHIP_TRANSFERRED] = "OwnershipTransferred"
}

type Query struct {
	SQL        string
	BindValues interface{}
}

type AzimuthEventLog struct {
	BlockNumber uint64      `db:"block_number"`
	BlockHash   common.Hash `db:"block_hash"`
	TxHash      common.Hash `db:"tx_hash"`
	LogIndex    uint        `db:"log_index"`

	ContractAddress common.Address `db:"contract_address"`
	Name            string         `db:"name"`
	Topic0          common.Hash    `db:"topic0"` // Hashed version of Name and the arg types
	Topic1          common.Hash    `db:"topic1"`
	Topic2          common.Hash    `db:"topic2"`
	Data            []byte         `db:"data"`

	IsProcessed bool `db:"is_processed"`
}

func (db *DB) Save(e AzimuthEventLog) {
	fmt.Printf("%#v\n", e)
	_, err := db.DB.NamedExec(`
		insert into event_logs (
			            block_number, block_hash, tx_hash, log_index, contract_address, topic0, topic1, topic2, data, is_processed
			        ) values (
			            :block_number, :block_hash, :tx_hash, :log_index, :contract_address, :topic0, :topic1, :topic2, :data,
			            :is_processed
			        )
	`, e)
	if err != nil {
		panic(err)
	}
}

func (db *DB) PlayAzimuthLogs() {
	var events []AzimuthEventLog
	for {
		// Batches of 500
		err := db.DB.Select(&events, `
			select block_number, block_hash, tx_hash, log_index, contract_address, topic0, topic1,
			        topic2, data, is_processed from event_logs where is_processed = 0 order by block_number, log_index asc limit 500
		`)
		if err != nil {
			panic(err)
		} else if len(events) == 0 {
			// No unprocessed logs left; we're finished
			break
		}
		db.ApplyEventEffects(events)
	}
}

func (db *DB) ApplyEventEffects(events []AzimuthEventLog) {
	tx, err := db.DB.Begin()
	if err != nil {
		panic(err)
	}

	for _, e := range events {
		effects := e.Effects()

		if effects.SQL != "" {
			_, err = db.DB.NamedExec(effects.SQL, effects.BindValues)
			if err != nil {
				fmt.Printf("%q; %#v\n", effects.SQL, effects.BindValues)
				if err := tx.Rollback(); err != nil {
					panic(err)
				}
				panic(err)
			}
		}

		_, err = db.DB.NamedExec(`update event_logs set is_processed=1 where block_number = :block_number and log_index = :log_index`, e)
		if err != nil {
			if err := tx.Rollback(); err != nil {
				panic(err)
			}
			panic(err)
		}
	}
	if err = tx.Commit(); err != nil {
		panic(err)
	}
}

func topic_to_uint32(h common.Hash) uint32 {
	// Topics are 32 bytes; uint32s are 4 bytes; so remove the first 28
	return binary.BigEndian.Uint32([]byte(h[28:]))
}
func topic_to_azimuth_number(h common.Hash) AzimuthNumber {
	return AzimuthNumber(topic_to_uint32(h))
}
func topic_to_eth_address(h common.Hash) common.Address {
	// Topics are 32 bytes; addresses are 20; so remove the first 12
	return common.BytesToAddress(h[:])
}

func (e AzimuthEventLog) Effects() Query {
	switch e.Topic0 {
	case SPAWNED:
		p := Point{
			Number: topic_to_azimuth_number(e.Topic2),
		}
		return Query{`insert into points (azimuth_number) values (:azimuth_number)`, p}
	case ACTIVATED:
		p := Point{
			Number:     topic_to_azimuth_number(e.Topic1),
			IsActive:   true,
			HasSponsor: true,
		}
		if p.Number < 0x10000 {
			// A star's original sponsor is a galaxy
			p.Sponsor = p.Number % 0x100
		} else {
			// A planet's original sponsor is a star
			p.Sponsor = p.Number % 0x10000
		}
		return Query{`
			insert into points (azimuth_number, is_active, has_sponsor, sponsor)
			            values (:azimuth_number, :is_active, :has_sponsor, :sponsor)
			on conflict do update
			        set is_active=:is_active,
			            has_sponsor=:has_sponsor,
			            sponsor=:sponsor`,
			p,
		}
	case OWNER_CHANGED:
		p := Point{
			Number:       topic_to_azimuth_number(e.Topic1),
			OwnerAddress: topic_to_eth_address(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, owner_address)
						values (:azimuth_number, :owner_address)
			on conflict do update
			        set owner_address=:owner_address`,
			p,
		}
	case CHANGED_SPAWN_PROXY:
		p := Point{
			Number:       topic_to_azimuth_number(e.Topic1),
			SpawnAddress: topic_to_eth_address(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, spawn_address)
			            values (:azimuth_number, :spawn_address)
			on conflict do update
			        set spawn_address=:spawn_address`,
			p,
		}
	case CHANGED_TRANSFER_PROXY:
		p := Point{
			Number:          topic_to_azimuth_number(e.Topic1),
			TransferAddress: topic_to_eth_address(e.Topic2),
		}
		return Query{
			`insert into points (azimuth_number, transfer_address)
			             values (:azimuth_number, :transfer_address)
			 on conflict do update
			         set transfer_address=:transfer_address`,
			p,
		}
	case CHANGED_MANAGEMENT_PROXY:
		p := Point{
			Number:            topic_to_azimuth_number(e.Topic1),
			ManagementAddress: topic_to_eth_address(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, management_address)
			            values (:azimuth_number, :management_address)
			on conflict do update
			        set management_address=:management_address`,
			p,
		}
	case CHANGED_VOTING_PROXY:
		p := Point{
			Number:        topic_to_azimuth_number(e.Topic1),
			VotingAddress: topic_to_eth_address(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, voting_address)
			            values (:azimuth_number, :voting_address)
			on conflict do update
			        set voting_address=:voting_address`,
			p,
		}
	case ESCAPE_REQUESTED:
		p := Point{
			Number:            topic_to_azimuth_number(e.Topic1),
			IsEscapeRequested: true,
			EscapeRequestedTo: topic_to_azimuth_number(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, is_escape_requested, escape_requested_to)
			            values (:azimuth_number, 1, :escape_requested_to)
			on conflict do update
			        set is_escape_requested=1,
			            escape_requested_to=:escape_requested_to`,
			p,
		}
	case ESCAPE_CANCELED:
		p := Point{
			Number:            topic_to_azimuth_number(e.Topic1),
			IsEscapeRequested: false,
			EscapeRequestedTo: AzimuthNumber(0),
		}
		return Query{`
			insert into points (azimuth_number, is_escape_requested, escape_requested_to)
			            values (:azimuth_number, 0, 0)
			on conflict do update
			        set is_escape_requested=0, escape_requested_to=0`,
			p,
		}
	case ESCAPE_ACCEPTED:
		p := Point{
			Number:            topic_to_azimuth_number(e.Topic1),
			IsEscapeRequested: false,
			EscapeRequestedTo: AzimuthNumber(0),
			HasSponsor:        true,
			Sponsor:           topic_to_azimuth_number(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, is_escape_requested, escape_requested_to, has_sponsor, sponsor)
			            values (:azimuth_number, :is_escape_requested, :escape_requested_to, :has_sponsor, :sponsor)
		    on conflict do update
		            set is_escape_requested=:is_escape_requested,
		                escape_requested_to=:escape_requested_to,
		                has_sponsor=:has_sponsor,
		                sponsor=:sponsor`,
			p,
		}
	case LOST_SPONSOR:
		p := Point{
			Number:     topic_to_azimuth_number(e.Topic1),
			HasSponsor: false,
		}
		return Query{`
			insert into points (azimuth_number, has_sponsor)
			            values (:azimuth_number, :has_sponsor)
			on conflict do update
			        set has_sponsor=:has_sponsor`,
			p,
		}
	case BROKE_CONTINUITY:
		p := Point{
			Number: topic_to_azimuth_number(e.Topic1),
			Rift:   topic_to_uint32(e.Topic2),
		}
		return Query{`
			insert into points (azimuth_number, rift)
			            values (:azimuth_number, :rift)
			on conflict do update
			        set rift=:rift`,
			p,
		}
	case CHANGED_KEYS:
		if len(e.Data) != 32*4 { // Four 32-byte EVM words
			panic(len(e.Data))
		}
		p := Point{
			Number:             topic_to_azimuth_number(e.Topic1),
			EncryptionKey:      e.Data[:32],
			AuthKey:            e.Data[32 : 32*2],
			CryptoSuiteVersion: binary.BigEndian.Uint32([]byte(e.Data[32*3-4 : 32*3])),
			Life:               binary.BigEndian.Uint32([]byte(e.Data[32*4-4 : 32*4])),
		}
		fmt.Printf("CryptoSuiteVersion: %#v\n", p)
		return Query{`
			insert into points (azimuth_number, encryption_key, auth_key, crypto_suite_version, life)
			            values (:azimuth_number, :encryption_key, :auth_key, :crypto_suite_version, :life)
			on conflict do update
			        set encryption_key=:encryption_key,
			            auth_key=:auth_key,
			            crypto_suite_version=:crypto_suite_version,
			            life=:life`,
			p,
		}
	case CHANGED_DNS:
		return Query{} // TODO
	default:
		panic("???")
	}
}
