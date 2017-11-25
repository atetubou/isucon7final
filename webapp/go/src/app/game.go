package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strconv"
	"sync/atomic"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/websocket"
	"github.com/jmoiron/sqlx"
)

type GameRequest struct {
	RequestID int    `json:"request_id"`
	Action    string `json:"action"`
	Time      int64  `json:"time"`

	// for addIsu
	Isu string `json:"isu"`

	// for buyItem
	ItemID      int `json:"item_id"`
	CountBought int `json:"count_bought"`
}

type GameResponse struct {
	RequestID int  `json:"request_id"`
	IsSuccess bool `json:"is_success"`
}

// 10進数の指数表記に使うデータ。JSONでは [仮数部, 指数部] という2要素配列になる。
type Exponential struct {
	// Mantissa * 10 ^ Exponent
	Mantissa int64
	Exponent int64
}

func (n Exponential) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%d,%d]", n.Mantissa, n.Exponent)), nil
}

type Adding struct {
	RoomName string `json:"-" db:"room_name"`
	Time     int64  `json:"time" db:"time"`
	Isu      string `json:"isu" db:"isu"`
}

type Buying struct {
	RoomName string `db:"room_name"`
	ItemID   int    `db:"item_id"`
	Ordinal  int    `db:"ordinal"`
	Time     int64  `db:"time"`
}

type Schedule struct {
	Time       int64       `json:"time"`
	MilliIsu   Exponential `json:"milli_isu"`
	TotalPower Exponential `json:"total_power"`
}

type Item struct {
	ItemID      int         `json:"item_id"`
	CountBought int         `json:"count_bought"`
	CountBuilt  int         `json:"count_built"`
	NextPrice   Exponential `json:"next_price"`
	Power       Exponential `json:"power"`
	Building    []Building  `json:"building"`
}

type OnSale struct {
	ItemID int   `json:"item_id"`
	Time   int64 `json:"time"`
}

type Building struct {
	Time       int64       `json:"time"`
	CountBuilt int         `json:"count_built"`
	Power      Exponential `json:"power"`
}

type GameStatus struct {
	Time     int64      `json:"time"`
	Adding   []Adding   `json:"adding"`
	Schedule []Schedule `json:"schedule"`
	Items    []Item     `json:"items"`
	OnSale   []OnSale   `json:"on_sale"`
}

type mItem struct {
	ItemID int   `db:"item_id"`
	Power1 int64 `db:"power1"`
	Power2 int64 `db:"power2"`
	Power3 int64 `db:"power3"`
	Power4 int64 `db:"power4"`
	Price1 int64 `db:"price1"`
	Price2 int64 `db:"price2"`
	Price3 int64 `db:"price3"`
	Price4 int64 `db:"price4"`
}

type itemPre struct {
	power     *big.Int
	power2exp Exponential

	price     *big.Int
	price2exp Exponential
	price1000 *big.Int
}

var buyCnt uint64
var addCnt uint64
var statusCnt uint64

var mItems = []mItem{}
var precalced = [][]itemPre{}

var sen = big.NewInt(1000)

func getPower(m mItem, itemID, cnt int) *big.Int {
	if len(precalced) == 0 {
		return m.GetPower(cnt)
	}
	p := precalced[itemID-1]
	if cnt < len(p) {
		return p[cnt].power
	}
	return m.GetPower(cnt)
}

func getPrice(m mItem, itemID, cnt int) *big.Int {
	if len(precalced) == 0 {
		return m.GetPrice(cnt)
	}
	p := precalced[itemID-1]
	if cnt < len(p) {
		return p[cnt].price
	}
	return m.GetPrice(cnt)
}

func getPrice1000(m mItem, itemID, cnt int) *big.Int {
	if len(precalced) == 0 {
		return new(big.Int).Mul(m.GetPrice(cnt), sen)
	}
	p := precalced[itemID-1]
	if cnt < len(p) {
		return p[cnt].price1000
	}
	return new(big.Int).Mul(m.GetPrice(cnt), sen)
}

func getPrice2exp(m mItem, itemID, cnt int) Exponential {
	if len(precalced) == 0 {
		return big2exp(m.GetPrice(cnt))
	}
	p := precalced[itemID-1]
	if cnt < len(p) {
		return p[cnt].price2exp
	}
	return big2exp(m.GetPrice(cnt))
}

func PrecalcItems() {
	tx, err := db.Beginx()
	if err != nil {
		log.Fatal(err)
	}

	sen := big.NewInt(1000)

	limit := []int{500, 200, 200, 200, 100, 100, 100, 50, 50, 50, 30, 30, 10}

	for i := 0; i < 13; i++ {
		var item mItem
		tx.Get(&item, "SELECT * FROM m_item WHERE item_id = ?", i+1)
		mItems = append(mItems, item)
		items := []itemPre{}
		for j := 0; j < limit[i]; j++ {
			power := item.GetPower(j)
			price := item.GetPrice(j)
			new(big.Int).Mul(power, sen)
			items = append(items, itemPre{
				power:     power,
				power2exp: big2exp(power),

				price:     price,
				price2exp: big2exp(price),
				price1000: new(big.Int).Mul(price, sen),
			})
		}
		precalced = append(precalced, items)
	}

	err = tx.Commit()
	if err != nil {
		log.Fatal(err)
	}
}

func (item *mItem) GetPower(count int) *big.Int {
	// power(x):=(cx+1)*d^(ax+b)
	a := item.Power1
	b := item.Power2
	c := item.Power3
	d := item.Power4
	x := int64(count)

	s := big.NewInt(c*x + 1)
	t := new(big.Int).Exp(big.NewInt(d), big.NewInt(a*x+b), nil)
	return new(big.Int).Mul(s, t)
}

func (item *mItem) GetPrice(count int) *big.Int {
	// price(x):=(cx+1)*d^(ax+b)
	a := item.Price1
	b := item.Price2
	c := item.Price3
	d := item.Price4
	x := int64(count)

	s := big.NewInt(c*x + 1)
	t := new(big.Int).Exp(big.NewInt(d), big.NewInt(a*x+b), nil)
	return new(big.Int).Mul(s, t)
}

func str2big(s string) *big.Int {
	x := new(big.Int)
	x.SetString(s, 10)
	return x
}

func big2exp(n *big.Int) Exponential {
	if n.IsInt64() {
		i64 := n.Int64()
		if i64 < 1000000000000000 {
			return Exponential{i64, 0}
		}
	}
	s := n.String()

	if len(s) <= 15 {
		return Exponential{n.Int64(), 0}
	}

	t, err := strconv.ParseInt(s[:15], 10, 64)
	if err != nil {
		log.Panic(err)
	}
	return Exponential{t, int64(len(s) - 15)}
}

func getCurrentTime() (int64, error) {
	var currentTime int64
	err := db.Get(&currentTime, "SELECT floor(unix_timestamp(current_timestamp(3))*1000)")
	if err != nil {
		return 0, err
	}
	return currentTime, nil
}

// 部屋のロックを取りタイムスタンプを更新する
//
// トランザクション開始後この関数を呼ぶ前にクエリを投げると、
// そのトランザクション中の通常のSELECTクエリが返す結果がロック取得前の
// 状態になることに注意 (keyword: MVCC, repeatable read).
func updateRoomTime(tx *sqlx.Tx, roomName string, reqTime int64) (int64, bool) {
	// See page 13 and 17 in https://www.slideshare.net/ichirin2501/insert-51938787
	_, err := tx.Exec("INSERT INTO room_time(room_name, time) VALUES (?, 0) ON DUPLICATE KEY UPDATE time = time", roomName)
	if err != nil {
		log.Println(err)
		return 0, false
	}

	var roomTime int64
	err = tx.Get(&roomTime, "SELECT time FROM room_time WHERE room_name = ? FOR UPDATE", roomName)
	if err != nil {
		log.Println(err)
		return 0, false
	}

	var currentTime int64
	err = tx.Get(&currentTime, "SELECT floor(unix_timestamp(current_timestamp(3))*1000)")
	if err != nil {
		log.Println(err)
		return 0, false
	}
	if roomTime > currentTime {
		log.Println("room time is future")
		return 0, false
	}
	if reqTime != 0 {
		if reqTime < currentTime {
			log.Println("reqTime is past")
			return 0, false
		}
	}

	_, err = tx.Exec("UPDATE room_time SET time = ? WHERE room_name = ?", currentTime, roomName)
	if err != nil {
		log.Println(err)
		return 0, false
	}

	return currentTime, true
}

func addIsu(roomName string, reqIsu *big.Int, reqTime int64) bool {
	tx, err := db.Beginx()
	if err != nil {
		log.Println(err)
		return false
	}

	_, ok := updateRoomTime(tx, roomName, reqTime)
	if !ok {
		tx.Rollback()
		return false
	}

	_, err = tx.Exec("INSERT INTO adding(room_name, time, isu) VALUES (?, ?, '0') ON DUPLICATE KEY UPDATE isu=isu", roomName, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	var isuStr string
	err = tx.QueryRow("SELECT isu FROM adding WHERE room_name = ? AND time = ? FOR UPDATE", roomName, reqTime).Scan(&isuStr)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}
	isu := str2big(isuStr)

	isu.Add(isu, reqIsu)
	_, err = tx.Exec("UPDATE adding SET isu = ? WHERE room_name = ? AND time = ?", isu.String(), roomName, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Println(err)
		return false
	}
	return true
}

func buyItem(roomName string, itemID int, countBought int, reqTime int64) bool {
	tx, err := db.Beginx()
	if err != nil {
		log.Println(err)
		return false
	}

	_, ok := updateRoomTime(tx, roomName, reqTime)
	if !ok {
		tx.Rollback()
		return false
	}

	var countBuying int
	err = tx.Get(&countBuying, "SELECT COUNT(*) FROM buying WHERE room_name = ? AND item_id = ?", roomName, itemID)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}
	if countBuying != countBought {
		tx.Rollback()
		log.Println(roomName, itemID, countBought+1, " is already bought")
		return false
	}

	totalMilliIsu := new(big.Int)
	var addings []Adding
	err = tx.Select(&addings, "SELECT isu FROM adding WHERE room_name = ? AND time <= ?", roomName, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	for _, a := range addings {
		totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), sen))
	}

	var buyings []Buying
	err = tx.Select(&buyings, "SELECT item_id, ordinal, time FROM buying WHERE room_name = ?", roomName)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}
	for _, b := range buyings {
		var item mItem = mItems[b.ItemID-1]
		cost := getPrice1000(item, b.ItemID, b.Ordinal)
		totalMilliIsu.Sub(totalMilliIsu, cost)
		if b.Time <= reqTime {
			gain := new(big.Int).Mul(getPower(item, b.ItemID, b.Ordinal), big.NewInt(reqTime-b.Time))
			totalMilliIsu.Add(totalMilliIsu, gain)
		}
	}

	var item mItem = mItems[itemID-1]

	need := getPrice1000(item, itemID, countBought+1)
	if totalMilliIsu.Cmp(need) < 0 {
		log.Println("not enough")
		tx.Rollback()
		return false
	}

	_, err = tx.Exec("INSERT INTO buying(room_name, item_id, ordinal, time) VALUES(?, ?, ?, ?)", roomName, itemID, countBought+1, reqTime)
	if err != nil {
		log.Println(err)
		tx.Rollback()
		return false
	}

	if err := tx.Commit(); err != nil {
		log.Println(err)
		return false
	}

	return true
}

func getStatus(roomName string) (*GameStatus, error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}

	currentTime, ok := updateRoomTime(tx, roomName, 0)
	if !ok {
		tx.Rollback()
		return nil, fmt.Errorf("updateRoomTime failure")
	}
	tx.Commit()

	mItems := map[int]mItem{}
	var items []mItem
	err = db.Select(&items, "SELECT * FROM m_item")
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		mItems[item.ItemID] = item
	}

	addings := []Adding{}
	err = db.Select(&addings, "SELECT time, isu FROM adding WHERE room_name = ?", roomName)
	if err != nil {
		return nil, err
	}

	buyings := []Buying{}
	err = db.Select(&buyings, "SELECT item_id, ordinal, time FROM buying WHERE room_name = ?", roomName)
	if err != nil {
		return nil, err
	}

	status, err := calcStatus(currentTime, mItems, addings, buyings)
	if err != nil {
		return nil, err
	}

	// calcStatusに時間がかかる可能性があるので タイムスタンプを取得し直す
	latestTime, err := getCurrentTime()
	if err != nil {
		return nil, err
	}

	status.Time = latestTime
	return status, err
}

func calcStatus(currentTime int64, mItems map[int]mItem, addings []Adding, buyings []Buying) (*GameStatus, error) {
	var (
		// 1ミリ秒に生産できる椅子の単位をミリ椅子とする
		totalMilliIsu = big.NewInt(0)
		totalPower    = big.NewInt(0)

		itemPower    = map[int]*big.Int{}    // ItemID => Power
		itemPrice    = map[int]*big.Int{}    // ItemID => Price
		itemOnSale   = map[int]int64{}       // ItemID => OnSale
		itemBuilt    = map[int]int{}         // ItemID => BuiltCount
		itemBought   = map[int]int{}         // ItemID => CountBought
		itemBuilding = map[int][]Building{}  // ItemID => Buildings
		itemPower0   = map[int]Exponential{} // ItemID => currentTime における Power
		itemBuilt0   = map[int]int{}         // ItemID => currentTime における BuiltCount

		addingAt = map[int64]Adding{}   // Time => currentTime より先の Adding
		buyingAt = map[int64][]Buying{} // Time => currentTime より先の Buying
	)

	for itemID := range mItems {
		itemPower[itemID] = big.NewInt(0)
		itemBuilding[itemID] = []Building{}
	}

	for _, a := range addings {
		// adding は adding.time に isu を増加させる
		if a.Time <= currentTime {
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), big.NewInt(1000)))
		} else {
			addingAt[a.Time] = a
		}
	}

	for _, b := range buyings {
		// buying は 即座に isu を消費し buying.time からアイテムの効果を発揮する
		itemBought[b.ItemID]++
		m := mItems[b.ItemID]

		price1000 := getPrice1000(m, b.ItemID, b.Ordinal)

		totalMilliIsu.Sub(totalMilliIsu, price1000)

		if b.Time <= currentTime {
			itemBuilt[b.ItemID]++
			power := getPower(m, b.ItemID, itemBought[b.ItemID])
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(power, big.NewInt(currentTime-b.Time)))
			totalPower.Add(totalPower, power)
			itemPower[b.ItemID].Add(itemPower[b.ItemID], power)
		} else {
			buyingAt[b.Time] = append(buyingAt[b.Time], b)
		}
	}

	sen := big.NewInt(1000)
	senbl := sen.BitLen()
	totalbl := totalMilliIsu.BitLen()
	for _, m := range mItems {
		itemPower0[m.ItemID] = big2exp(itemPower[m.ItemID])
		itemBuilt0[m.ItemID] = itemBuilt[m.ItemID]
		price := getPrice(m, m.ItemID, itemBought[m.ItemID]+1)
		itemPrice[m.ItemID] = price
		const offset = 3
		pbl := price.BitLen()
		if totalbl > pbl+senbl+offset {
			itemOnSale[m.ItemID] = 0
			continue
		}

		if totalbl+offset < pbl+senbl {
			continue
		}

		// totalMilliIsu >= price * sen?
		if 0 <= totalMilliIsu.Cmp(new(big.Int).Mul(price, sen)) {
			itemOnSale[m.ItemID] = 0 // 0 は 時刻 currentTime で購入可能であることを表す
		}
	}

	schedule := []Schedule{
		Schedule{
			Time:       currentTime,
			MilliIsu:   big2exp(totalMilliIsu),
			TotalPower: big2exp(totalPower),
		},
	}

	// currentTime から 1000 ミリ秒先までシミュレーションする
	for t := currentTime + 1; t <= currentTime+1000; t++ {
		totalMilliIsu.Add(totalMilliIsu, totalPower)
		updated := false

		// 時刻 t で発生する adding を計算する
		if a, ok := addingAt[t]; ok {
			updated = true
			totalMilliIsu.Add(totalMilliIsu, new(big.Int).Mul(str2big(a.Isu), big.NewInt(1000)))
		}

		// 時刻 t で発生する buying を計算する
		if _, ok := buyingAt[t]; ok {
			updated = true
			updatedID := map[int]bool{}
			for _, b := range buyingAt[t] {
				m := mItems[b.ItemID]
				updatedID[b.ItemID] = true
				itemBuilt[b.ItemID]++
				power := getPower(m, b.ItemID, b.Ordinal)
				itemPower[b.ItemID].Add(itemPower[b.ItemID], power)
				totalPower.Add(totalPower, power)
			}
			for id := range updatedID {
				itemBuilding[id] = append(itemBuilding[id], Building{
					Time:       t,
					CountBuilt: itemBuilt[id],
					Power:      big2exp(itemPower[id]),
				})
			}
		}

		if updated {
			schedule = append(schedule, Schedule{
				Time:       t,
				MilliIsu:   big2exp(totalMilliIsu),
				TotalPower: big2exp(totalPower),
			})
		}

		totalBitlen := totalMilliIsu.BitLen()
		senBitlen := sen.BitLen()

		// 時刻 t で購入可能になったアイテムを記録する
		for itemID := range mItems {
			if _, ok := itemOnSale[itemID]; ok {
				continue
			}

			const offset = 3

			ib := itemPrice[itemID].BitLen()
			if totalBitlen > ib+senBitlen+offset {
				itemOnSale[itemID] = t
				continue
			}

			if totalBitlen+offset < ib+senBitlen {
				continue
			}

			if 0 <= totalMilliIsu.Cmp(new(big.Int).Mul(itemPrice[itemID], sen)) {
				itemOnSale[itemID] = t
			}
		}
	}

	gsAdding := []Adding{}
	for _, a := range addingAt {
		gsAdding = append(gsAdding, a)
	}

	gsItems := []Item{}
	for itemID, m := range mItems {

		gsItems = append(gsItems, Item{
			ItemID:      itemID,
			CountBought: itemBought[itemID],
			CountBuilt:  itemBuilt0[itemID],
			NextPrice:   getPrice2exp(m, itemID, itemBought[m.ItemID]+1),
			Power:       itemPower0[itemID],
			Building:    itemBuilding[itemID],
		})
	}

	gsOnSale := []OnSale{}
	for itemID, t := range itemOnSale {
		gsOnSale = append(gsOnSale, OnSale{
			ItemID: itemID,
			Time:   t,
		})
	}

	return &GameStatus{
		Adding:   gsAdding,
		Schedule: schedule,
		Items:    gsItems,
		OnSale:   gsOnSale,
	}, nil
}

func serveGameConn(ws *websocket.Conn, roomName string) {
	log.Println(ws.RemoteAddr(), "serveGameConn", roomName)
	defer ws.Close()

	status, err := getStatus(roomName)
	if err != nil {
		log.Println(err)
		return
	}

	err = ws.WriteJSON(status)
	if err != nil {
		log.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	chReq := make(chan GameRequest)

	go func() {
		defer cancel()
		for {
			req := GameRequest{}
			err := ws.ReadJSON(&req)
			if err != nil {
				log.Println(err)
				return
			}

			select {
			case chReq <- req:
			case <-ctx.Done():
				return
			}
		}
	}()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case req := <-chReq:
			log.Println(req)

			success := false
			switch req.Action {
			case "addIsu":
				success = addIsu(roomName, str2big(req.Isu), req.Time)
				atomic.AddUint64(&addCnt, 1)
			case "buyItem":
				success = buyItem(roomName, req.ItemID, req.CountBought, req.Time)
				atomic.AddUint64(&buyCnt, 1)
			default:
				log.Println("Invalid Action")
				return
			}

			if success {
				// GameResponse を返却する前に 反映済みの GameStatus を返す
				status, err := getStatus(roomName)
				if err != nil {
					log.Println(err)
					return
				}

				err = ws.WriteJSON(status)
				if err != nil {
					log.Println(err)
					return
				}
			}

			err := ws.WriteJSON(GameResponse{
				RequestID: req.RequestID,
				IsSuccess: success,
			})
			if err != nil {
				log.Println(err)
				return
			}
		case <-ticker.C:
			atomic.AddUint64(&statusCnt, 1)
			status, err := getStatus(roomName)
			if err != nil {
				log.Println(err)
				return
			}

			err = ws.WriteJSON(status)
			if err != nil {
				log.Println(err)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
