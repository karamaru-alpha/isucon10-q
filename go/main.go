package main

import (
	"bytes"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	goLog "log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/sonic/decoder"
	"github.com/bytedance/sonic/encoder"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const Limit = 20
const NazotteLimit = 50

var db *sqlx.DB
var mySQLConnectionData *MySQLConnectionEnv
var chairSearchCondition ChairSearchCondition
var estateSearchCondition EstateSearchCondition

type InitializeResponse struct {
	Language string `json:"language"`
}

type Chair struct {
	ID          int64  `db:"id" json:"id"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
	Thumbnail   string `db:"thumbnail" json:"thumbnail"`
	Price       int64  `db:"price" json:"price"`
	Height      int64  `db:"height" json:"height"`
	Width       int64  `db:"width" json:"width"`
	Depth       int64  `db:"depth" json:"depth"`
	Color       string `db:"color" json:"color"`
	Features    string `db:"features" json:"features"`
	Kind        string `db:"kind" json:"kind"`
	Popularity  int64  `db:"popularity" json:"-"`
	Stock       int64  `db:"stock" json:"-"`
}

type ChairSearchResponse struct {
	Count  int64   `json:"count"`
	Chairs []Chair `json:"chairs"`
}

type ChairListResponse struct {
	Chairs []Chair `json:"chairs"`
}

//Estate 物件
type Estate struct {
	ID          int64   `db:"id" json:"id"`
	Thumbnail   string  `db:"thumbnail" json:"thumbnail"`
	Name        string  `db:"name" json:"name"`
	Description string  `db:"description" json:"description"`
	Latitude    float64 `db:"latitude" json:"latitude"`
	Longitude   float64 `db:"longitude" json:"longitude"`
	Address     string  `db:"address" json:"address"`
	Rent        int64   `db:"rent" json:"rent"`
	DoorHeight  int64   `db:"door_height" json:"doorHeight"`
	DoorWidth   int64   `db:"door_width" json:"doorWidth"`
	Features    string  `db:"features" json:"features"`
	Popularity  int64   `db:"popularity" json:"-"`
}

//EstateSearchResponse estate/searchへのレスポンスの形式
type EstateSearchResponse struct {
	Count   int64    `json:"count"`
	Estates []Estate `json:"estates"`
}

type EstateListResponse struct {
	Estates []Estate `json:"estates"`
}

type Coordinate struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Coordinates struct {
	Coordinates []Coordinate `json:"coordinates"`
}

type Range struct {
	ID  int64 `json:"id"`
	Min int64 `json:"min"`
	Max int64 `json:"max"`
}

type RangeCondition struct {
	Prefix string   `json:"prefix"`
	Suffix string   `json:"suffix"`
	Ranges []*Range `json:"ranges"`
}

type ListCondition struct {
	List []string `json:"list"`
}

type EstateSearchCondition struct {
	DoorWidth  RangeCondition `json:"doorWidth"`
	DoorHeight RangeCondition `json:"doorHeight"`
	Rent       RangeCondition `json:"rent"`
	Feature    ListCondition  `json:"feature"`
}

type ChairSearchCondition struct {
	Width   RangeCondition `json:"width"`
	Height  RangeCondition `json:"height"`
	Depth   RangeCondition `json:"depth"`
	Price   RangeCondition `json:"price"`
	Color   ListCondition  `json:"color"`
	Feature ListCondition  `json:"feature"`
	Kind    ListCondition  `json:"kind"`
}

type BoundingBox struct {
	// TopLeftCorner 緯度経度が共に最小値になるような点の情報を持っている
	TopLeftCorner Coordinate
	// BottomRightCorner 緯度経度が共に最大値になるような点の情報を持っている
	BottomRightCorner Coordinate
}

type MySQLConnectionEnv struct {
	Host     string
	Port     string
	User     string
	DBName   string
	Password string
}

type RecordMapper struct {
	Record []string

	offset int
	err    error
}

func (r *RecordMapper) next() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.offset >= len(r.Record) {
		r.err = fmt.Errorf("too many read")
		return "", r.err
	}
	s := r.Record[r.offset]
	r.offset++
	return s, nil
}

func (r *RecordMapper) NextInt() int {
	s, err := r.next()
	if err != nil {
		goLog.Println(err)
		return 0
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		goLog.Println(err)
		r.err = err
		return 0
	}
	return i
}

func (r *RecordMapper) NextFloat() float64 {
	s, err := r.next()
	if err != nil {
		goLog.Println(err)
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		goLog.Println(err)
		r.err = err
		return 0
	}
	return f
}

func (r *RecordMapper) NextString() string {
	s, err := r.next()
	if err != nil {
		goLog.Println(err)
		return ""
	}
	return s
}

func (r *RecordMapper) Err() error {
	return r.err
}

func NewMySQLConnectionEnv() *MySQLConnectionEnv {
	return &MySQLConnectionEnv{
		Host:     getEnv("MYSQL_HOST", "127.0.0.1"),
		Port:     getEnv("MYSQL_PORT", "3306"),
		User:     getEnv("MYSQL_USER", "isucon"),
		DBName:   getEnv("MYSQL_DBNAME", "isuumo"),
		Password: getEnv("MYSQL_PASS", "isucon"),
	}
}

func getEnv(key, defaultValue string) string {
	val := os.Getenv(key)
	if val != "" {
		return val
	}
	return defaultValue
}

//ConnectDB isuumoデータベースに接続する
func (mc *MySQLConnectionEnv) ConnectDB() (*sqlx.DB, error) {
	dsn := fmt.Sprintf("%v:%v@tcp(%v:%v)/%v?interpolateParams=true", mc.User, mc.Password, mc.Host, mc.Port, mc.DBName)
	return sqlx.Open("mysql", dsn)
}

func init() {
	jsonText, err := ioutil.ReadFile("../fixture/chair_condition.json")
	if err != nil {
		goLog.Println(err)
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	json.Unmarshal(jsonText, &chairSearchCondition)

	jsonText, err = ioutil.ReadFile("../fixture/estate_condition.json")
	if err != nil {
		goLog.Println(err)
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
	json.Unmarshal(jsonText, &estateSearchCondition)

	http.DefaultTransport.(*http.Transport).MaxIdleConns = 0
	http.DefaultTransport.(*http.Transport).MaxIdleConnsPerHost = 4096
	http.DefaultTransport.(*http.Transport).ForceAttemptHTTP2 = true
}

type JSONSerializer struct{}

func (j *JSONSerializer) Serialize(c echo.Context, i interface{}, indent string) error {
	buf, err := encoder.Encode(i, 0)
	if err != nil {
		return err
	}
	_, err = c.Response().Write(buf)
	return err
}

func (j *JSONSerializer) Deserialize(c echo.Context, i interface{}) error {
	buf := new(bytes.Buffer)
	buf.ReadFrom(c.Request().Body)
	err := decoder.NewDecoder(buf.String()).Decode(i)
	if ute, ok := err.(*json.UnmarshalTypeError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Unmarshal type error: expected=%v, got=%v, field=%v, offset=%v", ute.Type, ute.Value, ute.Field, ute.Offset)).SetInternal(err)
	} else if se, ok := err.(*json.SyntaxError); ok {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Syntax error: offset=%v, error=%v", se.Offset, se.Error())).SetInternal(err)
	}
	return err
}

type omLowPriceChairT struct {
	M sync.RWMutex
	V []Chair
}

var omLowPriceChair omLowPriceChairT

func (o *omLowPriceChairT) Get() []Chair {
	o.M.RLock()
	defer o.M.RUnlock()
	return o.V
}

func (o *omLowPriceChairT) Set(v []Chair) {
	o.M.Lock()
	o.V = v
	o.M.Unlock()
}

type omLowPriceEstateT struct {
	M sync.RWMutex
	V []Estate
}

var omLowPriceEstate omLowPriceEstateT

func (o *omLowPriceEstateT) Get() []Estate {
	o.M.RLock()
	defer o.M.RUnlock()
	return o.V
}

func (o *omLowPriceEstateT) Set(v []Estate) {
	o.M.Lock()
	o.V = v
	o.M.Unlock()
}

func main() {
	// TODO
	goLog.SetFlags(goLog.Lshortfile)
	logfile, err := os.OpenFile("/var/log/go.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		goLog.Println(err)
		panic("cannnot open test.log:" + err.Error())
	}
	defer logfile.Close()
	goLog.SetOutput(io.MultiWriter(logfile, os.Stdout))

	// Echo instance
	e := echo.New()
	e.JSONSerializer = &JSONSerializer{}

	// Middleware
	e.Use(middleware.Recover())

	// Initialize
	e.POST("/initialize", initialize)

	// Chair Handler
	// * path
	e.GET("/api/chair/:id", getChairDetail)
	e.POST("/api/chair", postChair)
	e.GET("/api/chair/search", searchChairs)
	e.GET("/api/chair/low_priced", getLowPricedChair)
	e.GET("/api/chair/search/condition", getChairSearchCondition)
	e.POST("/api/chair/buy/:id", buyChair)

	// Estate Handler
	e.GET("/api/estate/:id", getEstateDetail)
	e.POST("/api/estate", postEstate)
	e.GET("/api/estate/search", searchEstates)
	e.GET("/api/estate/low_priced", getLowPricedEstate)
	e.POST("/api/estate/req_doc/:id", postEstateRequestDocument)
	e.POST("/api/estate/nazotte", searchEstateNazotte)
	e.GET("/api/estate/search/condition", getEstateSearchCondition)
	e.GET("/api/recommended_estate/:id", searchRecommendedEstateWithChair)

	// Unix Domain Socket
	socketFile := "/tmp/app.sock"
	os.Remove(socketFile)
	l, err := net.Listen("unix", socketFile)
	if err != nil {
		goLog.Println(err)
		e.Logger.Fatal(err)
	}
	err = os.Chmod(socketFile, 0777)
	if err != nil {
		goLog.Println(err)
		e.Logger.Fatal(err)
	}
	e.Listener = l

	mySQLConnectionData = NewMySQLConnectionEnv()

	db, err = mySQLConnectionData.ConnectDB()
	if err != nil {
		goLog.Println(err)
		e.Logger.Fatalf("DB connection failed : %v", err)
	}
	defer db.Close()
	const SQL_CONN_COUNT = 40
	// 最大接続数
	db.SetMaxOpenConns(SQL_CONN_COUNT)
	// プールできるコネクションの数
	db.SetMaxIdleConns(SQL_CONN_COUNT)
	// 接続が確立されてからコネクションを保持できる最大時間
	db.SetConnMaxLifetime(SQL_CONN_COUNT * time.Second)
	for {
		err := db.Ping()
		if err == nil {
			break
		}
		time.Sleep(time.Second * 1)
	}

	// Start server
	serverPort := fmt.Sprintf(":%v", getEnv("SERVER_PORT", "1323"))
	e.Logger.Fatal(e.Start(serverPort))
}

func initialize(c echo.Context) error {
	goLog.Println("initialize!")
	sqlDir := filepath.Join("..", "mysql", "db")
	paths := []string{
		filepath.Join(sqlDir, "0_Schema.sql"),
		filepath.Join(sqlDir, "1_DummyEstateData.sql"),
		filepath.Join(sqlDir, "2_DummyChairData.sql"),
		filepath.Join(sqlDir, "3.sql"),
	}

	for _, p := range paths {
		sqlFile, _ := filepath.Abs(p)
		cmdStr := fmt.Sprintf("mysql -h %v -u %v -p%v -P %v %v < %v",
			mySQLConnectionData.Host,
			mySQLConnectionData.User,
			mySQLConnectionData.Password,
			mySQLConnectionData.Port,
			mySQLConnectionData.DBName,
			sqlFile,
		)
		if err := exec.Command("bash", "-c", cmdStr).Run(); err != nil {
			goLog.Println(err)
			c.Logger().Errorf("Initialize script error : %v", err)
			return c.NoContent(http.StatusInternalServerError)
		}
	}

	var chairs []Chair
	db.Select(&chairs, "SELECT * FROM chair WHERE stock > 0 ORDER BY price ASC, id ASC LIMIT ?", Limit)
	omLowPriceChair.Set(chairs)

	var estates []Estate
	db.Select(&estates, "SELECT * FROM estate ORDER BY rent ASC, id ASC LIMIT ?", Limit)
	omLowPriceEstate.Set(estates)

	return c.JSON(http.StatusOK, InitializeResponse{
		Language: "go",
	})
}

func getChairDetail(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		goLog.Println(err)
		c.Echo().Logger.Errorf("Request parameter \"id\" parse error : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	chair := Chair{}
	query := `SELECT * FROM chair WHERE id = ?`
	err = db.Get(&chair, query, id)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("requested id's chair not found : %v", id)
			return c.NoContent(http.StatusNotFound)
		}
		c.Echo().Logger.Errorf("Failed to get the chair from id : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	} else if chair.Stock <= 0 {
		c.Echo().Logger.Infof("requested id's chair is sold out : %v", id)
		return c.NoContent(http.StatusNotFound)
	}

	return c.JSON(http.StatusOK, chair)
}

func postChair(c echo.Context) error {
	header, err := c.FormFile("chairs")
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to get form file: %v", err)
		return c.NoContent(http.StatusBadRequest)
	}
	f, err := header.Open()
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to open form file: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to read csv: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	args := make([]interface{}, 0, len(records)*13)
	placeHolders := &strings.Builder{}
	for i, row := range records {
		rm := RecordMapper{Record: row}
		id := rm.NextInt()
		name := rm.NextString()
		description := rm.NextString()
		thumbnail := rm.NextString()
		price := rm.NextInt()
		height := rm.NextInt()
		width := rm.NextInt()
		depth := rm.NextInt()
		color := rm.NextString()
		features := rm.NextString()
		kind := rm.NextString()
		popularity := rm.NextInt()
		stock := rm.NextInt()
		if err := rm.Err(); err != nil {
			c.Logger().Errorf("failed to read record: %v", err)
			return c.NoContent(http.StatusBadRequest)
		}
		args = append(args, id, name, description, thumbnail, price, height, width, depth, color, features, kind, popularity, stock)
		if i == 0 {
			placeHolders.WriteString(" (?,?,?,?,?,?,?,?,?,?,?,?,null,?)")
		} else {
			placeHolders.WriteString(",(?,?,?,?,?,?,?,?,?,?,?,?,null,?)")
		}
	}
	_, err = db.Exec("INSERT INTO chair(id, name, description, thumbnail, price, height, width, depth, color, features, kind, popularity, popularity_desc, stock) VALUES"+placeHolders.String(), args...)
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to insert chair: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var chairs []Chair
	db.Select(&chairs, "SELECT * FROM chair WHERE stock > 0 ORDER BY price ASC, id ASC LIMIT ?", Limit)
	omLowPriceChair.Set(chairs)

	return c.NoContent(http.StatusCreated)
}

func searchChairs(c echo.Context) error {
	conditions := make([]string, 0)
	params := make([]interface{}, 0)

	if c.QueryParam("priceRangeId") != "" {
		chairPrice, err := getRange(chairSearchCondition.Price, c.QueryParam("priceRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("priceRangeID invalid, %v : %v", c.QueryParam("priceRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairPrice.Min != -1 {
			conditions = append(conditions, "price >= ?")
			params = append(params, chairPrice.Min)
		}
		if chairPrice.Max != -1 {
			conditions = append(conditions, "price < ?")
			params = append(params, chairPrice.Max)
		}
	}

	if c.QueryParam("heightRangeId") != "" {
		chairHeight, err := getRange(chairSearchCondition.Height, c.QueryParam("heightRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("heightRangeIf invalid, %v : %v", c.QueryParam("heightRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairHeight.Min != -1 {
			conditions = append(conditions, "height >= ?")
			params = append(params, chairHeight.Min)
		}
		if chairHeight.Max != -1 {
			conditions = append(conditions, "height < ?")
			params = append(params, chairHeight.Max)
		}
	}

	if c.QueryParam("widthRangeId") != "" {
		chairWidth, err := getRange(chairSearchCondition.Width, c.QueryParam("widthRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("widthRangeID invalid, %v : %v", c.QueryParam("widthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairWidth.Min != -1 {
			conditions = append(conditions, "width >= ?")
			params = append(params, chairWidth.Min)
		}
		if chairWidth.Max != -1 {
			conditions = append(conditions, "width < ?")
			params = append(params, chairWidth.Max)
		}
	}

	if c.QueryParam("depthRangeId") != "" {
		chairDepth, err := getRange(chairSearchCondition.Depth, c.QueryParam("depthRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("depthRangeId invalid, %v : %v", c.QueryParam("depthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if chairDepth.Min != -1 {
			conditions = append(conditions, "depth >= ?")
			params = append(params, chairDepth.Min)
		}
		if chairDepth.Max != -1 {
			conditions = append(conditions, "depth < ?")
			params = append(params, chairDepth.Max)
		}
	}

	if c.QueryParam("kind") != "" {
		conditions = append(conditions, "kind = ?")
		params = append(params, c.QueryParam("kind"))
	}

	if c.QueryParam("color") != "" {
		conditions = append(conditions, "color = ?")
		params = append(params, c.QueryParam("color"))
	}

	if c.QueryParam("features") != "" {
		for _, f := range strings.Split(c.QueryParam("features"), ",") {
			conditions = append(conditions, "features LIKE CONCAT('%', ?, '%')")
			params = append(params, f)
		}
	}

	if len(conditions) == 0 {
		c.Echo().Logger.Infof("Search condition not found")
		return c.NoContent(http.StatusBadRequest)
	}

	conditions = append(conditions, "stock > 0")

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		goLog.Println(err)
		c.Logger().Infof("Invalid format page parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	perPage, err := strconv.Atoi(c.QueryParam("perPage"))
	if err != nil {
		goLog.Println(err)
		c.Logger().Infof("Invalid format perPage parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	searchQuery := "SELECT * FROM chair WHERE "
	countQuery := "SELECT COUNT(*) FROM chair WHERE "
	searchCondition := strings.Join(conditions, " AND ")
	limitOffset := " ORDER BY popularity_desc, id ASC LIMIT ? OFFSET ?"

	var res ChairSearchResponse
	err = db.Get(&res.Count, countQuery+searchCondition, params...)
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("searchChairs DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	chairs := []Chair{}
	params = append(params, perPage, page*perPage)
	err = db.Select(&chairs, searchQuery+searchCondition+limitOffset, params...)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, ChairSearchResponse{Count: 0, Chairs: []Chair{}})
		}
		c.Logger().Errorf("searchChairs DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res.Chairs = chairs

	return c.JSON(http.StatusOK, res)
}

// * score
func buyChair(c echo.Context) error {
	m := echo.Map{}
	if err := c.Bind(&m); err != nil {
		c.Echo().Logger.Infof("post buy chair failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, ok := m["email"].(string)
	if !ok {
		c.Echo().Logger.Info("post buy chair failed : email not found in request body")
		return c.NoContent(http.StatusBadRequest)
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		goLog.Println(err)
		c.Echo().Logger.Infof("post buy chair failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	// tx, err := db.Beginx()
	// if err != nil {
	// 	goLog.Println(err)
	// 	c.Echo().Logger.Errorf("failed to create transaction : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }
	// defer tx.Rollback()

	// var chair Chair
	// err = tx.QueryRowx("SELECT * FROM chair WHERE id = ? AND stock > 0 FOR UPDATE", id).StructScan(&chair)
	// if err != nil {
	// 	goLog.Println(err)
	// 	if err == sql.ErrNoRows {
	// 		c.Echo().Logger.Infof("buyChair chair id \"%v\" not found", id)
	// 		return c.NoContent(http.StatusNotFound)
	// 	}
	// 	c.Echo().Logger.Errorf("DB Execution Error: on getting a chair by id : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	_, err = db.Exec("UPDATE chair SET stock = stock - 1 WHERE id = ? AND stock > 0", id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("buyChair chair id \"%v\" not found", id)
			return c.NoContent(http.StatusNotFound)
		}
		goLog.Println(err)
		c.Echo().Logger.Errorf("chair stock update failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	// err = tx.Commit()
	// if err != nil {
	// 	goLog.Println(err)
	// 	c.Echo().Logger.Errorf("transaction commit error : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	return c.NoContent(http.StatusOK)
}

func getChairSearchCondition(c echo.Context) error {
	return c.JSON(http.StatusOK, chairSearchCondition)
}

func getLowPricedChair(c echo.Context) error {
	// var chairs []Chair
	// query := `SELECT * FROM chair WHERE stock > 0 ORDER BY price ASC, id ASC LIMIT ?`
	// err := db.Select(&chairs, query, Limit)
	// if err != nil {
	// 	goLog.Println(err)
	// 	if err == sql.ErrNoRows {
	// 		c.Logger().Error("getLowPricedChair not found")
	// 		return c.JSON(http.StatusOK, ChairListResponse{[]Chair{}})
	// 	}
	// 	c.Logger().Errorf("getLowPricedChair DB execution error : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	chairs := omLowPriceChair.Get()
	return c.JSON(http.StatusOK, ChairListResponse{Chairs: chairs})
}

func getEstateDetail(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		goLog.Println(err)
		c.Echo().Logger.Infof("Request parameter \"id\" parse error : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	var estate Estate
	err = db.Get(&estate, "SELECT * FROM estate WHERE id = ?", id)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			c.Echo().Logger.Infof("getEstateDetail estate id %v not found", id)
			return c.NoContent(http.StatusNotFound)
		}
		c.Echo().Logger.Errorf("Database Execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, estate)
}

func getRange(cond RangeCondition, rangeID string) (*Range, error) {
	RangeIndex, err := strconv.Atoi(rangeID)
	if err != nil {
		goLog.Println(err)
		return nil, err
	}

	if RangeIndex < 0 || len(cond.Ranges) <= RangeIndex {
		return nil, fmt.Errorf("Unexpected Range ID")
	}

	return cond.Ranges[RangeIndex], nil
}

func postEstate(c echo.Context) error {
	header, err := c.FormFile("estates")
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to get form file: %v", err)
		return c.NoContent(http.StatusBadRequest)
	}
	f, err := header.Open()
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to open form file: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to read csv: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	args := make([]interface{}, 0, len(records)*13)
	placeHolders := &strings.Builder{}
	for i, row := range records {
		rm := RecordMapper{Record: row}
		id := rm.NextInt()
		name := rm.NextString()
		description := rm.NextString()
		thumbnail := rm.NextString()
		address := rm.NextString()
		latitude := rm.NextFloat()
		longitude := rm.NextFloat()
		rent := rm.NextInt()
		doorHeight := rm.NextInt()
		doorWidth := rm.NextInt()
		features := rm.NextString()
		popularity := rm.NextInt()
		if err := rm.Err(); err != nil {
			c.Logger().Errorf("failed to read record: %v", err)
			return c.NoContent(http.StatusBadRequest)
		}
		geom := fmt.Sprintf("POINT(%f %f)", latitude, longitude)
		args = append(args, []interface{}{id, name, description, thumbnail, address, latitude, longitude, rent, doorHeight, doorWidth, features, popularity, geom}...)
		if i == 0 {
			placeHolders.WriteString(" (?,?,?,?,?,?,?,?,?,?,?,?,null,ST_PointFromText(?))")
		} else {
			placeHolders.WriteString(",(?,?,?,?,?,?,?,?,?,?,?,?,null,ST_PointFromText(?))")
		}
	}
	_, err = db.Exec("INSERT INTO estate(id, name, description, thumbnail, address, latitude, longitude, rent, door_height, door_width, features, popularity, popularity_desc, geom) VALUES"+placeHolders.String(), args...)
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("failed to insert estate: %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var estates []Estate
	db.Select(&estates, "SELECT * FROM estate ORDER BY rent ASC, id ASC LIMIT ?", Limit)
	omLowPriceEstate.Set(estates)

	return c.NoContent(http.StatusCreated)
}

func searchEstates(c echo.Context) error {
	conditions := make([]string, 0)
	params := make([]interface{}, 0)

	if c.QueryParam("doorHeightRangeId") != "" {
		doorHeight, err := getRange(estateSearchCondition.DoorHeight, c.QueryParam("doorHeightRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("doorHeightRangeID invalid, %v : %v", c.QueryParam("doorHeightRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if doorHeight.Min != -1 {
			conditions = append(conditions, "door_height >= ?")
			params = append(params, doorHeight.Min)
		}
		if doorHeight.Max != -1 {
			conditions = append(conditions, "door_height < ?")
			params = append(params, doorHeight.Max)
		}
	}

	if c.QueryParam("doorWidthRangeId") != "" {
		doorWidth, err := getRange(estateSearchCondition.DoorWidth, c.QueryParam("doorWidthRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("doorWidthRangeID invalid, %v : %v", c.QueryParam("doorWidthRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if doorWidth.Min != -1 {
			conditions = append(conditions, "door_width >= ?")
			params = append(params, doorWidth.Min)
		}
		if doorWidth.Max != -1 {
			conditions = append(conditions, "door_width < ?")
			params = append(params, doorWidth.Max)
		}
	}

	if c.QueryParam("rentRangeId") != "" {
		estateRent, err := getRange(estateSearchCondition.Rent, c.QueryParam("rentRangeId"))
		if err != nil {
			goLog.Println(err)
			c.Echo().Logger.Infof("rentRangeID invalid, %v : %v", c.QueryParam("rentRangeId"), err)
			return c.NoContent(http.StatusBadRequest)
		}

		if estateRent.Min != -1 {
			conditions = append(conditions, "rent >= ?")
			params = append(params, estateRent.Min)
		}
		if estateRent.Max != -1 {
			conditions = append(conditions, "rent < ?")
			params = append(params, estateRent.Max)
		}
	}

	if c.QueryParam("features") != "" {
		for _, f := range strings.Split(c.QueryParam("features"), ",") {
			conditions = append(conditions, "features like concat('%', ?, '%')")
			params = append(params, f)
		}
	}

	if len(conditions) == 0 {
		c.Echo().Logger.Infof("searchEstates search condition not found")
		return c.NoContent(http.StatusBadRequest)
	}

	page, err := strconv.Atoi(c.QueryParam("page"))
	if err != nil {
		goLog.Println(err)
		c.Logger().Infof("Invalid format page parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	perPage, err := strconv.Atoi(c.QueryParam("perPage"))
	if err != nil {
		goLog.Println(err)
		c.Logger().Infof("Invalid format perPage parameter : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	searchQuery := "SELECT * FROM estate WHERE "
	countQuery := "SELECT COUNT(*) FROM estate WHERE "
	searchCondition := strings.Join(conditions, " AND ")
	limitOffset := " ORDER BY popularity_desc, id ASC LIMIT ? OFFSET ?"

	var res EstateSearchResponse
	err = db.Get(&res.Count, countQuery+searchCondition, params...)
	if err != nil {
		goLog.Println(err)
		c.Logger().Errorf("searchEstates DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	estates := []Estate{}
	params = append(params, perPage, page*perPage)
	err = db.Select(&estates, searchQuery+searchCondition+limitOffset, params...)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, EstateSearchResponse{Count: 0, Estates: []Estate{}})
		}
		c.Logger().Errorf("searchEstates DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	res.Estates = estates

	return c.JSON(http.StatusOK, res)
}

func getLowPricedEstate(c echo.Context) error {
	// estates := make([]Estate, 0, Limit)
	// query := `SELECT * FROM estate ORDER BY rent ASC, id ASC LIMIT ?`
	// err := db.Select(&estates, query, Limit)
	// if err != nil {
	// 	if err == sql.ErrNoRows {
	// 		c.Logger().Error("getLowPricedEstate not found")
	// 		return c.JSON(http.StatusOK, EstateListResponse{[]Estate{}})
	// 	}
	// 	goLog.Println(err)
	// 	c.Logger().Errorf("getLowPricedEstate DB execution error : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }
	estates := omLowPriceEstate.Get()
	return c.JSON(http.StatusOK, EstateListResponse{Estates: estates})
}

func searchRecommendedEstateWithChair(c echo.Context) error {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		goLog.Println(err)
		c.Logger().Infof("Invalid format searchRecommendedEstateWithChair id : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	chair := Chair{}
	query := `SELECT * FROM chair WHERE id = ?`
	err = db.Get(&chair, query, id)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			c.Logger().Infof("Requested chair id \"%v\" not found", id)
			return c.NoContent(http.StatusBadRequest)
		}
		c.Logger().Errorf("Database execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	var estates []Estate
	w := chair.Width
	h := chair.Height
	d := chair.Depth
	query = `SELECT * FROM estate WHERE (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) OR (door_width >= ? AND door_height >= ?) ORDER BY popularity_desc, id ASC LIMIT ?`
	err = db.Select(&estates, query, w, h, w, d, h, w, h, d, d, w, d, h, Limit)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, EstateListResponse{[]Estate{}})
		}
		c.Logger().Errorf("Database execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.JSON(http.StatusOK, EstateListResponse{Estates: estates})
}

// ! now
func searchEstateNazotte(c echo.Context) error {
	coordinates := Coordinates{}
	err := c.Bind(&coordinates)
	if err != nil {
		goLog.Println(err)
		c.Echo().Logger.Infof("post search estate nazotte failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	if len(coordinates.Coordinates) == 0 {
		return c.NoContent(http.StatusBadRequest)
	}

	// b := coordinates.getBoundingBox()
	// estatesInBoundingBox := []Estate{}
	// query := `SELECT * FROM estate WHERE latitude <= ? AND latitude >= ? AND longitude <= ? AND longitude >= ? ORDER BY popularity_desc, id ASC`
	// err = db.Select(&estatesInBoundingBox, query, b.BottomRightCorner.Latitude, b.TopLeftCorner.Latitude, b.BottomRightCorner.Longitude, b.TopLeftCorner.Longitude)
	// if err == sql.ErrNoRows {
	// 	c.Echo().Logger.Infof("select * from estate where latitude ...", err)
	// 	return c.JSON(http.StatusOK, EstateSearchResponse{Count: 0, Estates: []Estate{}})
	// } else if err != nil {
	// 	goLog.Println(err)
	// 	c.Echo().Logger.Errorf("database execution error : %v", err)
	// 	return c.NoContent(http.StatusInternalServerError)
	// }

	estatesInPolygon := []Estate{}
	// for _, estate := range estatesInBoundingBox {
	// 	validatedEstate := Estate{}

	// 	point := fmt.Sprintf("'POINT(%f %f)'", estate.Latitude, estate.Longitude)
	// 	query := fmt.Sprintf(`SELECT * FROM estate WHERE id = ? AND ST_Contains(ST_PolygonFromText(%s), ST_GeomFromText(%s))`, coordinates.coordinatesToText(), point)
	// 	err = db.Get(&validatedEstate, query, estate.ID)
	// 	if err != nil {
	// 		if err == sql.ErrNoRows {
	// 			continue
	// 		} else {
	// 			goLog.Println(err)
	// 			c.Echo().Logger.Errorf("db access is failed on executing validate if estate is in polygon : %v", err)
	// 			return c.NoContent(http.StatusInternalServerError)
	// 		}
	// 	} else {
	// 		estatesInPolygon = append(estatesInPolygon, validatedEstate)
	// 	}
	// }
	query := fmt.Sprintf("SELECT * FROM estate WHERE ST_Contains(ST_PolygonFromText(%s), geom) ORDER BY popularity_desc, id ASC LIMIT %d", coordinates.coordinatesToText(), NazotteLimit)
	err = db.Select(&estatesInPolygon, query)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusOK, EstateSearchResponse{Count: 0})
		}
		goLog.Println(err)
		c.Echo().Logger.Errorf("db access is failed on executing validate if estate is in polygon : %v", err)
		return c.NoContent(http.StatusInternalServerError)

	}

	var re EstateSearchResponse
	re.Estates = []Estate{}
	re.Estates = estatesInPolygon
	re.Count = int64(len(re.Estates))

	return c.JSON(http.StatusOK, re)
}

// * score
func postEstateRequestDocument(c echo.Context) error {
	m := echo.Map{}
	if err := c.Bind(&m); err != nil {
		c.Echo().Logger.Infof("post request document failed : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	_, ok := m["email"].(string)
	if !ok {
		c.Echo().Logger.Info("post request document failed : email not found in request body")
		return c.NoContent(http.StatusBadRequest)
	}

	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		goLog.Println(err)
		c.Echo().Logger.Infof("post request document failed : %v", err)
		return c.NoContent(http.StatusBadRequest)
	}

	estate := Estate{}
	query := `SELECT * FROM estate WHERE id = ?`
	err = db.Get(&estate, query, id)
	if err != nil {
		goLog.Println(err)
		if err == sql.ErrNoRows {
			return c.NoContent(http.StatusNotFound)
		}
		c.Logger().Errorf("postEstateRequestDocument DB execution error : %v", err)
		return c.NoContent(http.StatusInternalServerError)
	}

	return c.NoContent(http.StatusOK)
}

func getEstateSearchCondition(c echo.Context) error {
	return c.JSON(http.StatusOK, estateSearchCondition)
}

func (cs Coordinates) getBoundingBox() BoundingBox {
	coordinates := cs.Coordinates
	boundingBox := BoundingBox{
		TopLeftCorner: Coordinate{
			Latitude: coordinates[0].Latitude, Longitude: coordinates[0].Longitude,
		},
		BottomRightCorner: Coordinate{
			Latitude: coordinates[0].Latitude, Longitude: coordinates[0].Longitude,
		},
	}
	for _, coordinate := range coordinates {
		if boundingBox.TopLeftCorner.Latitude > coordinate.Latitude {
			boundingBox.TopLeftCorner.Latitude = coordinate.Latitude
		}
		if boundingBox.TopLeftCorner.Longitude > coordinate.Longitude {
			boundingBox.TopLeftCorner.Longitude = coordinate.Longitude
		}

		if boundingBox.BottomRightCorner.Latitude < coordinate.Latitude {
			boundingBox.BottomRightCorner.Latitude = coordinate.Latitude
		}
		if boundingBox.BottomRightCorner.Longitude < coordinate.Longitude {
			boundingBox.BottomRightCorner.Longitude = coordinate.Longitude
		}
	}
	return boundingBox
}

func (cs Coordinates) coordinatesToText() string {
	points := make([]string, 0, len(cs.Coordinates))
	for _, c := range cs.Coordinates {
		points = append(points, fmt.Sprintf("%f %f", c.Latitude, c.Longitude))
	}
	return fmt.Sprintf("'POLYGON((%s))'", strings.Join(points, ","))
}
