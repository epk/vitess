package binlog

import (
	"reflect"
	"testing"

	"golang.org/x/net/context"

	"github.com/youtube/vitess/go/mysqlconn/replication"
	"github.com/youtube/vitess/go/vt/sqlparser"
	"github.com/youtube/vitess/go/vt/vttablet/tabletserver/schema"

	binlogdatapb "github.com/youtube/vitess/go/vt/proto/binlogdata"
	querypb "github.com/youtube/vitess/go/vt/proto/query"
)

// This file tests the RBR events are parsed correctly.

func TestStreamerParseRBRUpdateEvent(t *testing.T) {
	f := replication.NewMySQL56BinlogFormat()
	s := replication.NewFakeBinlogStream()
	s.ServerID = 62344

	// Create a schema.Engine for this test, with just one table.
	// We only use the Columns.
	se := schema.NewEngineForTests()
	se.SetTableForTests(&schema.Table{
		Name: sqlparser.NewTableIdent("vt_a"),
		Columns: []schema.TableColumn{
			{
				Name: sqlparser.NewColIdent("id"),
				Type: querypb.Type_INT64,
			},
			{
				Name: sqlparser.NewColIdent("message"),
				Type: querypb.Type_VARCHAR,
			},
		},
	})

	// Create a tableMap event on the table.
	tableID := uint64(0x102030405060)
	tm := &replication.TableMap{
		Flags:    0x8090,
		Database: "vt_test_keyspace",
		Name:     "vt_a",
		Types: []byte{
			replication.TypeLong,
			replication.TypeVarchar,
		},
		CanBeNull: replication.NewServerBitmap(2),
		Metadata: []uint16{
			0,
			384, // A VARCHAR(128) in utf8 would result in 384.
		},
	}
	tm.CanBeNull.Set(1, true)

	// Do an update packet with all fields set.
	rows := replication.Rows{
		Flags:           0x1234,
		IdentifyColumns: replication.NewServerBitmap(2),
		DataColumns:     replication.NewServerBitmap(2),
		Rows: []replication.Row{
			{
				NullIdentifyColumns: replication.NewServerBitmap(2),
				NullColumns:         replication.NewServerBitmap(2),
				Identify: []byte{
					0x10, 0x20, 0x30, 0x40, // long
					0x03, 0x00, // len('abc')
					'a', 'b', 'c', // 'abc'
				},
				Data: []byte{
					0x10, 0x20, 0x30, 0x40, // long
					0x04, 0x00, // len('abcd')
					'a', 'b', 'c', 'd', // 'abcd'
				},
			},
		},
	}
	rows.IdentifyColumns.Set(0, true)
	rows.IdentifyColumns.Set(1, true)
	rows.DataColumns.Set(0, true)
	rows.DataColumns.Set(1, true)

	input := []replication.BinlogEvent{
		replication.NewRotateEvent(f, s, 0, ""),
		replication.NewFormatDescriptionEvent(f, s),
		replication.NewTableMapEvent(f, s, tableID, tm),
		replication.NewMariaDBGTIDEvent(f, s, replication.MariadbGTID{Domain: 0, Sequence: 0xd}, false /* hasBegin */),
		replication.NewQueryEvent(f, s, replication.Query{
			Database: "vt_test_keyspace",
			SQL:      "BEGIN"}),
		replication.NewUpdateRowsEvent(f, s, tableID, rows),
		replication.NewXIDEvent(f, s),
	}

	events := make(chan replication.BinlogEvent)

	want := []binlogdatapb.BinlogTransaction{
		{
			Statements: []*binlogdatapb.BinlogTransaction_Statement{
				{
					Category: binlogdatapb.BinlogTransaction_Statement_BL_SET,
					Sql:      []byte("SET TIMESTAMP=1407805592"),
				},
				{
					Category: binlogdatapb.BinlogTransaction_Statement_BL_UPDATE,
					Sql:      []byte("UPDATE vt_a SET id=1076895760, message='abcd' WHERE id=1076895760 AND message='abc'"),
				},
			},
			EventToken: &querypb.EventToken{
				Timestamp: 1407805592,
				Position: replication.EncodePosition(replication.Position{
					GTIDSet: replication.MariadbGTID{
						Domain:   0,
						Server:   62344,
						Sequence: 0x0d,
					},
				}),
			},
		},
	}
	var got []binlogdatapb.BinlogTransaction
	sendTransaction := func(trans *binlogdatapb.BinlogTransaction) error {
		got = append(got, *trans)
		return nil
	}
	bls := NewStreamer("vt_test_keyspace", nil, se, nil, replication.Position{}, 0, sendTransaction)

	go sendTestEvents(events, input)
	_, err := bls.parseEvents(context.Background(), events)
	if err != ErrServerEOF {
		t.Errorf("unexpected error: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("binlogConnStreamer.parseEvents(): got:\n%v\nwant:\n%v", got, want)
	}
}
