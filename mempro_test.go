package mempro

import (
    "fmt"
    "testing"
    "os"
    "os/exec"
    "time"
    "encoding/json"
)

// сколько ключей достаём за раз в бенчмарке мультигета
const multi_get_size = 100

// тестовые структуры
type Example struct {
    Num int
    Str string
}

type Example2 struct {
    Num2 int
    Str2 string
}

// сохраним запущеный мемкеш
// чтобы после тестов его стопнуть
var cmd *exec.Cmd


// возвращает имя тестового сокета
func getSocket() string {
    return fmt.Sprintf("/tmp/test-gomemcache-%d.sock", os.Getpid())
}


// запускает тестовый мемкеш на unix-сокете
func setupMemcache () error {
    sock := getSocket()
    cmd = exec.Command("memcached", "-s", sock)
    if err := cmd.Start(); err != nil {
        return err
    }

    // Wait a bit for the socket to appear.
    for i := 0; i < 10; i++ {
        if _, err := os.Stat(sock); err == nil {
            break
        }
        time.Sleep(time.Duration(25*i) * time.Millisecond)
    }

    // пришлось добавить ещё этот слип,
    // т.к. на моей машине иногда возникали сбои
    // обращения к сокету
    time.Sleep(1 * time.Second)

    return nil
}

// останавливает тестовый мемкеш
func stopMemcache () {
    //cmd.Wait()
    cmd.Process.Kill()
    os.Remove(getSocket())
}


// тест трёх основных методов: сет, гет, мультигет
func TestSetGetGetMulti(t *testing.T) {
    err := setupMemcache()
    if err != nil {
        t.Skip("skipping test; couldn't start memcached")
    }
    defer stopMemcache()
    mc := New(getSocket())

    // SetStruct
    tst := Example{Num: 123, Str: "abc"}
    err = mc.SetStruct("key_tst", tst)
    if err != nil {
        t.Fatal("SetStruct error:", err)
    }

    // GetStruct
    var tst_got Example
    err = mc.GetStruct("key_tst", &tst_got)
    if err != nil {
        t.Fatal("GetStruct error:", err)
    }
    if tst != tst_got {
        t.Error("Set and got objects are not the same: ", tst, tst_got)
    }

    // GetMultiStruct
    tst2 := Example2{Num2: 456, Str2: "def"}
    mc.SetStruct("key_tst2", tst2)
    var tst_multi Example
    var tst_multi2 Example2
    list := make(map[string]any)
    list["key_tst"] = &tst_multi
    list["key_tst2"] = &tst_multi2
    err = mc.GetMultiStruct(list)
    if err != nil {
        t.Fatal("GetMultiStruct error:", err)
    }
    tst_multi_got := list["key_tst"].(*Example)
    if tst != *tst_multi_got {
        t.Error("Set and got objects are not the same: ", tst, *tst_multi_got)
    }
    tst_multi_got2 := list["key_tst2"].(*Example2)
    if tst2 != *tst_multi_got2 {
        t.Error("Set and got objects (2) are not the same: ", tst2, *tst_multi_got2)
    }
}


func BenchmarkGetMulti(b *testing.B) {
    defer quiet()()

    err := setupMemcache()
    if err != nil {
        b.Skip("skipping test; couldn't start memcached")
    }
    defer stopMemcache()
    mc := New(getSocket())

    for i := 0; i < multi_get_size; i++ {
        x := Example{Num: i*100, Str: "abcdefgijklmnop"}
        key := fmt.Sprintf("testkey%d", i)
        err := mc.SetStruct(key, x)
        // здесь и далее в бенчмарках добавлена проверка ошибок
        // чтобы не оказалось, что мы что-то пытаемся измерить
        // на неработающем мемкеше
        if err != nil {
            b.Fatal(err)
        }
    }

    b.ResetTimer()
    for j := 0; j < b.N; j++ {
        b.StopTimer()
        mult := make(map[string]any)
        for i := 0; i < multi_get_size; i++ {
            var y Example
            key := fmt.Sprintf("testkey%d", i)
            mult[key] = &y
        }
        b.StartTimer()
        err := mc.GetMultiStruct(mult)
        if err != nil {
            b.Fatal(err)
        }
    }
}


func BenchmarkGetSingle(b *testing.B) {
    defer quiet()()

    mc := New("localhost:11211")

    for i := 0; i < multi_get_size; i++ {
        x := Example{Num: i*100, Str: "abcdefgijklmnop"}
        key := fmt.Sprintf("testkey%d", i)
        err := mc.SetStruct(key, x)
        if err != nil {
            b.Fatal(err)
        }
    }

    b.ResetTimer()
    for j := 0; j < b.N; j++ {
        b.StopTimer()
        for i := 0; i < multi_get_size; i++ {
            var y Example
            key := fmt.Sprintf("testkey%d", i)
            b.StartTimer()
            err := mc.GetStruct(key, &y)
            if err != nil {
                b.Fatal(err)
            }
        }
    }
}


// интересно понять, насколько стоило заморачиваться
// с распараллеливанием декодинга данных в GetMultiStruct
// метод мог получиться попроще, без каналов и горутин
// вот такой:
// (PS метод хоть и с большой буквы, но не экспортируется из тестов)
func (m *MemPro) GetMultiStructSimple(list map[string]any) error {
    keys := make([]string, len(list))
    i := 0
    for name := range list {
        keys[i] = name
        i++
    }
    items, err := m.GetMulti(keys)
    if err != nil {
        return err
    }
    for _, item := range items {
        err = json.Unmarshal(item.Value, list[item.Key])
        if err != nil {
            return err
        }
    }
    return nil
}

// и теперь бенчмарк с этим методом, аналогичный BenchmarkGetMulti
func BenchmarkGetMultiSimple(b *testing.B) {
    defer quiet()()

    err := setupMemcache()
    if err != nil {
        b.Skip("skipping test; couldn't start memcached")
    }
    defer stopMemcache()
    mc := New(getSocket())

    for i := 0; i < multi_get_size; i++ {
        x := Example{Num: i*100, Str: "abcdefgijklmnop"}
        key := fmt.Sprintf("testkey%d", i)
        err := mc.SetStruct(key, x)
        if err != nil {
            b.Fatal(err)
        }
    }

    b.ResetTimer()
    for j := 0; j < b.N; j++ {
        b.StopTimer()
        mult := make(map[string]any)
        for i := 0; i < multi_get_size; i++ {
            var y Example
            key := fmt.Sprintf("testkey%d", i)
            mult[key] = &y
        }
        b.StartTimer()
        err := mc.GetMultiStructSimple(mult)
        if err != nil {
            b.Fatal(err)
        }
    }
}

// на время бенчмарков скрываем разный мусорный вывод в stdout
func quiet() func() {
    null, _ := os.Open(os.DevNull)
    sout := os.Stdout
    serr := os.Stderr
    os.Stdout = null
    os.Stderr = null
    return func() {
        defer null.Close()
        os.Stdout = sout
        os.Stderr = serr
    }
}