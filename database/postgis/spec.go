package postgis

import (
	"fmt"
	"github.com/kucjac/imposm3/log"
	"reflect"
	"strconv"
	"strings"

	"github.com/kucjac/imposm3/mapping"
	"github.com/kucjac/imposm3/mapping/config"
	"github.com/pkg/errors"
)

type ColumnSpec struct {
	Name      string
	FieldType mapping.ColumnType
	Type      ColumnType
}
type TableSpec struct {
	Name            string
	FullName        string
	Schema          string
	Columns         []ColumnSpec
	GeometryType    string
	Srid            int
	Generalizations []*GeneralizedTableSpec
}

type GeneralizedTableSpec struct {
	Name              string
	FullName          string
	Schema            string
	SourceName        string
	Source            *TableSpec
	SourceGeneralized *GeneralizedTableSpec
	Tolerance         float64
	Where             string
	created           bool
	Generalizations   []*GeneralizedTableSpec
}

func (col *ColumnSpec) AsSQL() string {
	return fmt.Sprintf("\"%s\" %s", col.Name, col.Type.Name())
}

func (spec *TableSpec) CreateTableSQL() string {
	foundIdCol := false

	for _, cs := range spec.Columns {
		if cs.Name == "id" {
			foundIdCol = true
		}
	}

	cols := []string{}
	if !foundIdCol {
		// only add id column if there is no id configured
		// TODO allow to disable id column?
		cols = append(cols, "id SERIAL PRIMARY KEY")
	}

	for _, col := range spec.Columns {
		if col.Type.Name() == "GEOMETRY" {
			continue
		}
		cols = append(cols, col.AsSQL())
	}
	columnSQL := strings.Join(cols, ",\n")
	return fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS "%s"."%s" (
            %s
        );`,
		spec.Schema,
		spec.FullName,
		columnSQL,
	)
}

func (spec *TableSpec) InsertSQL() string {
	var cols []string
	var vars []string
	for _, col := range spec.Columns {
		cols = append(cols, "\""+col.Name+"\"")
		vars = append(vars,
			col.Type.PrepareInsertSql(len(vars)+1, spec))
	}
	columns := strings.Join(cols, ", ")
	placeholders := strings.Join(vars, ", ")

	return fmt.Sprintf(`INSERT INTO "%s"."%s" (%s) VALUES (%s)`,
		spec.Schema,
		spec.FullName,
		columns,
		placeholders,
	)
}

func (spec *TableSpec) CopySQL() string {
	var cols []string
	for _, col := range spec.Columns {
		cols = append(cols, "\""+col.Name+"\"")
	}
	columns := strings.Join(cols, ", ")

	return fmt.Sprintf(`COPY "%s"."%s" (%s) FROM STDIN`,
		spec.Schema,
		spec.FullName,
		columns,
	)
}

func (spec *TableSpec) DeleteSQL() string {
	var idColumnName string
	for _, col := range spec.Columns {
		if col.FieldType.Name == "id" {
			idColumnName = col.Name
			break
		}
	}

	if idColumnName == "" {
		panic("missing id column")
	}

	return fmt.Sprintf(`DELETE FROM "%s"."%s" WHERE "%s" = $1`,
		spec.Schema,
		spec.FullName,
		idColumnName,
	)
}

func NewTableSpec(pg *PostGIS, t *config.Table) (*TableSpec, error) {
	var geomType string
	if mapping.TableType(t.Type) == mapping.RelationMemberTable {
		geomType = "geometry"
	} else {
		geomType = string(t.Type)
	}

	spec := TableSpec{
		Name:         t.Name,
		FullName:     pg.Prefix + t.Name,
		Schema:       pg.Config.ImportSchema,
		GeometryType: geomType,
		Srid:         pg.Config.Srid,
	}

	for _, column := range t.Columns {
		columnType, err := mapping.MakeColumnType(column)
		if err != nil {
			return nil, err
		}

		pgType, ok := pgTypes[columnType.GoType]
		if !ok {
			return nil, errors.Errorf("unhandled column type %v, using string type", columnType)
			pgType = pgTypes["string"]
		}

		checkResolution := func(v int) int {
			if v > 15 {
				log.Printf("Table: '%s', Column: '%s'. Resolution is greater than possible value: '15'. Setting it to '15'. Value: '%v'", t.Name, column.Name, v)
				v = 15
			} else if v < 0 {
				log.Printf("Table: '%s', Column: '%s'. Resolution is lower than possible value: '0'. Setting it to '0'. Value: '%v'", t.Name, column.Name, v)
				v = 0
			}

			return v
		}

		h3, isH3Type := pgType.(*h3GeometryType)
		if isH3Type {
			res, ok := column.Args["resolutions"]
			if !ok {
				log.Printf("Table: %s, Column: %s. No indexed resolutions found. Indexing default resolution: '%v'", t.Name, column.Name, defaultH3Resolution)
				h3.indexedResolutions = append(h3.indexedResolutions, defaultH3Resolution)
			} else {
				switch r := res.(type) {
				case []interface{}:
					for _, i := range r {
						resolution, ok := i.(int)
						if !ok {
							return nil, errors.Errorf("One of the provided resoliutions is not an integer: %v.", i)
						}
						resolution = checkResolution(resolution)
						h3.indexedResolutions = append(h3.indexedResolutions, int8(resolution))

					}
				case []int:
					for i := range r {
						resolution := r[i]
						resolution = checkResolution(resolution)
						h3.indexedResolutions = append(h3.indexedResolutions, int8(resolution))
					}
				case int:
					res := checkResolution(r)
					h3.indexedResolutions = append(h3.indexedResolutions, int8(res))
				case string:
					intR, err := strconv.Atoi(r)
					if err != nil {
						return nil, errors.Errorf("Provided invalid resoultion: '%v' for the table: '%s', column: '%s'. Value is not an integer.", r, t.Name, column.Name)
					}
					h3.indexedResolutions = append(h3.indexedResolutions, int8(intR))
				case float64:
					v := int(r)
					v = checkResolution(v)
					h3.indexedResolutions = append(h3.indexedResolutions, int8(v))
				default:
					return nil, errors.Errorf("Provided invalid type ('%s') for the indexed resolutions for table: '%s', column: '%s'. ", reflect.TypeOf(res).String(), t.Name, column.Name)
				}
			}

		}

		col := ColumnSpec{column.Name, *columnType, pgType}
		spec.Columns = append(spec.Columns, col)
	}
	return &spec, nil
}

func NewGeneralizedTableSpec(pg *PostGIS, t *config.GeneralizedTable) *GeneralizedTableSpec {
	spec := GeneralizedTableSpec{
		Name:       t.Name,
		FullName:   pg.Prefix + t.Name,
		Schema:     pg.Config.ImportSchema,
		Tolerance:  t.Tolerance,
		Where:      t.SqlFilter,
		SourceName: t.SourceTableName,
	}
	return &spec
}

func (spec *GeneralizedTableSpec) DeleteSQL() string {
	var idColumnName string
	for _, col := range spec.Source.Columns {
		if col.FieldType.Name == "id" {
			idColumnName = col.Name
			break
		}
	}

	if idColumnName == "" {
		panic("missing id column")
	}

	return fmt.Sprintf(`DELETE FROM "%s"."%s" WHERE "%s" = $1`,
		spec.Schema,
		spec.FullName,
		idColumnName,
	)
}

func (spec *GeneralizedTableSpec) InsertSQL() string {
	var idColumnName string
	for _, col := range spec.Source.Columns {
		if col.FieldType.Name == "id" {
			idColumnName = col.Name
			break
		}
	}

	if idColumnName == "" {
		panic("missing id column")
	}

	var cols []string
	for _, col := range spec.Source.Columns {
		cols = append(cols, col.Type.GeneralizeSql(&col, spec))
	}

	where := fmt.Sprintf(` WHERE "%s" = $1`, idColumnName)
	if spec.Where != "" {
		where += " AND (" + spec.Where + ")"
	}

	columnSQL := strings.Join(cols, ",\n")
	sql := fmt.Sprintf(`INSERT INTO "%s"."%s" (SELECT %s FROM "%s"."%s"%s)`,
		spec.Schema, spec.FullName, columnSQL, spec.Source.Schema,
		spec.Source.FullName, where)
	return sql

}
