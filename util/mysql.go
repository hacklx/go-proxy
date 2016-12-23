package util

import (
	"database/sql"
	"log"

	_"github.com/go-sql-driver/mysql"
	"reflect"
	"fmt"
	"errors"
)

type Dbconfig struct {
	User string
	Pass string
	Ip   string
	Port int
	Dbname string
}

type Exec struct {
	db *sql.DB
	err error
}

type Orm struct {
	Rows *sql.Rows
	err error
}

func MysqlInit(user string ,pass string ,ip string, port int,dbname string)  *Dbconfig{
	MysqlClass := Dbconfig{user, pass, ip, port, dbname}
	return &MysqlClass
}

func (c *Dbconfig)Connect() *Exec{

	linkinfo := c.User + ":" + c.Pass + "@tcp(" + c.Ip + ":" + fmt.Sprintf("%d",c.Port) + ")/" + c.Dbname + ""

	db, err := sql.Open("mysql", linkinfo)

	if err != nil {
		return &Exec{nil,err}
	}

	err = db.Ping()
	if err != nil {
		return &Exec{nil,err}
	}

	return &Exec{db,nil}
}

func (r *Exec)Ping() error{
	if r.err != nil{
		return errors.New("数据库连接失败")
	}

	return nil
}

func (r *Exec)Close(){
	r.db.Close()
}

func (r *Exec)Query(sql string, args ...interface{}) bool {

	if r.db == nil {
		return false
	}

	stmt, err := r.db.Prepare(sql)

	defer stmt.Close()

	if err != nil {
		return false
	}

	stmt.Exec(args...)
	return true
}

func (r *Exec)QueryRow(sql string ,args ...interface{}) *sql.Row{
	return r.db.QueryRow(sql,args ...)
}

func (r *Exec)Select(sql string,args ...interface{})  *Orm{


	if r.err != nil{
		log.Println(r.err)
		return &Orm{nil,r.err}
	}
	defer r.db.Close()

	rows, err := r.db.Query(sql, args ...)
	if err != nil {
		return &Orm{nil,err}
	}

	return &Orm{rows,nil}
}


func (r *Orm)FetchAll(s interface{})*[]interface{}{

	result := make([]interface{}, 0)

	if r.err != nil{
		log.Println(r.err)
		return &result
	}

	defer r.Rows.Close()

	ref := reflect.ValueOf(s).Elem()

	arrays := make([]interface{}, ref.NumField())

	for i := 0; i < ref.NumField(); i++ {
		arrays[i] = ref.Field(i).Addr().Interface()
	}

	for r.Rows.Next(){

		err := r.Rows.Scan(arrays...)

		if err != nil{
			log.Println(err)
			return &result
		}

		result = append(result,ref.Interface())
	}
	return &result
}
