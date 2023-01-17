package mempro

import (
    "sync"
    "encoding/json"
    "github.com/bradfitz/gomemcache/memcache"
)

// размер буфера канала
// определяет, сколько параллельных потоков 
// расшифровки можно запустить в GetMultiStruct 
const buffered = 8

// наследник memcached
// только встроили родительскую структуру, новых полей нет
type MemPro struct {
    memcache.Client
}

// конструктор, возвращает клиент мемкеша
func New (server ...string) *MemPro {
    mc := memcache.New(server...)
    return &MemPro{Client: *mc}
}

// помещает в мемкеш структуру
// предварительно её кодирует с помощью json.Marshal
// третий необязательный параметр - TTL ключа в мемкеше
func (m *MemPro) SetStruct(key string, obj any, expiration ...int32) error {
    encoded, err := json.Marshal(obj)
    if err != nil {
        return err
    }
    item := memcache.Item{Key: key, Value: encoded}

    // определено ли время жизни ключа
    if len(expiration)>0 {
        item.Expiration = expiration[0]
    }
    err = m.Set(&item)
    return err
}

// получает из мемкеша структуру
func (m *MemPro) GetStruct(key string, obj any) error {
    item, err := m.Get(key)
    if err != nil {
        return err
    }
    err = json.Unmarshal(item.Value, obj)
    return err
}

// достаёт из мемкеша сразу несколько ключей за один запрос
// это оптимальнее, чем получать их по одному через одиночные Get-ы
// для ускорения, декодирование происходит в несколько потоков
func (m *MemPro) GetMultiStruct(list map[string]any) error {
    // почему здесь в качестве аргумента map а не *map
    // в go карта сама по себе содержит не значения, а ссылки на них
    // всё, что мы здесь сделаем со значениями list, будет видно вовне

    // составляем из ключей карты массив, который требуется на вход memcache.GetMulti
    keys := make([]string, len(list))
    i := 0
    for name := range list {
        keys[i] = name
        i++
    }
    // получаем зашифрованные данные из мемкеша
    items, err := m.GetMulti(keys)
    if err != nil {
        return err
    }
    // создаём канал. размер буфера определяет, 
    // сколько данных туда можно записать до наступления блокировки
    // по сути, сколько параллельных горутин будут заниматься расшифровкой
    ch := make(chan error, buffered)
    var lk sync.Mutex
    for _, item := range items {
        go func(val []byte, key string) {
            // любые обращения к карте в многопоточной среде
            // необходимо оборачивать в мьютекс
            // иначе свалимся с ошибкой concurrent map writes/reads
            lk.Lock()
            obj := list[key]
            lk.Unlock()
            // а вот декодирование должно быть вне блокировки
            // это самая ресурсозатратная операция тут, 
            // весь смысл данного огорода - её распараллелить
            ch <- json.Unmarshal(val, obj)
            lk.Lock()
            list[key] = obj
            lk.Unlock()
        }(item.Value, item.Key)
    }
    // проверяем, не свалились ли в канал ошибки
    for _ = range items {
        if ge := <-ch; ge != nil {
            err = ge
        }
    }
    return err
}
