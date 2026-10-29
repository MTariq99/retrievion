package document

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mtariq99/retrievion/internal/consts"
)

func TestRoutingKeyFor(t *testing.T) {
	rk, err := routingKeyFor(consts.EventTypeIngestionJobCreated)
	require.NoError(t, err)
	require.Equal(t, consts.IngestionRoutingKey, rk)

	_, err = routingKeyFor("something.unknown")
	require.Error(t, err)
}
