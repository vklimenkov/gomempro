package mempro

import (
    "encoding/json"
    "github.com/bradfitz/gomemcache/memcache"
)

// а это - наш рабочий объект, дочерний от memcached
type MemPro struct {
    memcache.Client // встроили структуру мемкеша
}

// конструктор
func NewMemPro (server ...string) *MemPro {
    mc := memcache.New(server...)
    //mcp := MemPro{Client: *mc}
    //return &mcp

    // то же самое в одну строку
    return &MemPro{Client: *mc}
}

func (m *MemPro) SetObj(k string, o any) error {
    encoded, err := json.Marshal(o)
    if err!=nil {
        return err
    }
    err = m.Set(&memcache.Item{Key: k, Value: encoded})
    return err
}
