package datasource

import (
	"context"
	"net/url"
	"strings"

	"github.com/hairyhenderson/gomplate/v3/coll"
	"github.com/hairyhenderson/gomplate/v3/internal/config"
	"github.com/hairyhenderson/gomplate/v3/internal/dataconv"

	"github.com/pkg/errors"
)

// Merge -
type Merge struct {
	reg SourceRegistry
}

var _ Reader = (*Merge)(nil)

// readMerge demultiplexes a `merge:` datasource. The 'args' parameter currently
// has no meaning for this source.
//
// URI format is 'merge:<source 1>|<source 2>[|<source n>...]' where `<source #>`
// is a supported URI or a pre-defined alias name.
//
// Query strings and fragments are interpreted relative to the merged data, not
// the source data. To merge datasources with query strings or fragments, define
// separate sources first and specify the alias names. HTTP headers are also not
// supported directly.
func (m *Merge) Read(ctx context.Context, url *url.URL, args ...string) (data *Data, err error) {
	opaque := url.Opaque
	parts := strings.Split(opaque, "|")
	if len(parts) < 2 {
		return nil, errors.New("need at least 2 datasources to merge")
	}
	data = newData(url, args)

	maps := make([]map[string]interface{}, len(parts))
	for i, part := range parts {
		// supports either URIs or aliases
		subSource := m.reg.Get(part)
		if subSource == nil {
			subSource, err = m.reg.Dynamic(part, nil)
			if err != nil {
				// maybe it's a relative filename?
				subSource, err = m.parseSource(part)
				if err != nil {
					return nil, err
				}
			}
		}

		subData, err := subSource.Read(ctx, args...)
		if err != nil {
			return nil, errors.Wrapf(err, "Couldn't read datasource '%s'", part)
		}

		maps[i], err = parseMap(subData)
		if err != nil {
			return nil, err
		}
	}

	// Merge the data together
	data.Bytes, err = mergeData(maps)
	if err != nil {
		return nil, err
	}

	data.MType = yamlMimetype
	return data, nil
}

func mergeData(data []map[string]interface{}) (out []byte, err error) {
	dst := data[0]
	data = data[1:]

	dst, err = coll.Merge(dst, data...)
	if err != nil {
		return nil, err
	}

	s, err := dataconv.ToYAML(dst)
	if err != nil {
		return nil, err
	}
	return []byte(s), nil
}

func parseMap(data *Data) (map[string]interface{}, error) {
	datum, err := data.Unmarshal()
	if err != nil {
		return nil, err
	}
	mimeType, err := data.MediaType()
	if err != nil {
		return nil, err
	}

	var m map[string]interface{}
	switch datum := datum.(type) {
	case map[string]interface{}:
		m = datum
	default:
		return nil, errors.Errorf("unexpected data type '%T' for datasource (type %s); merge: can only merge maps", datum, mimeType)
	}
	return m, nil
}

/////////////// ugh

// parseSource creates a *src by parsing the value provided to the
// --datasource/-d commandline flag
// TODO: rename this function
func (m *Merge) parseSource(value string) (Source, error) {
	var err error
	url, err := config.ParseSourceURL(value)
	if err != nil {
		return nil, err
	}
	return m.reg.Register(value, url, nil)
}