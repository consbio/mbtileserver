package handlers

import (
	"encoding/xml"
	"fmt"
	"github.com/beevik/etree"
	"net/http"
)

type Capabilities struct {
	XMLName xml.Name	`xml:"Capabilities"`
	Xmlns string 			`xml:"xmlns,attr"`
	Ows 	string			`xml:"xmlns:ows,attr"`
	Xlink string			`xml:"xmlns:xlink,attr"`
	Xsi		string			`xml:"xmlns:xsi,attr"`
	Gml		string			`xml:"xmlns:gml,attr"`
	SchemaLocation	string	`xml:"xmlns:schemaLocation,attr"`
	Version	string		`xml:"verion,attr"`
	ServiceIdentification				[]ServiceIdentification	`xml:"server"`
	OperationsMetadata	[]OperationsMetadata	`xml:"ows:OperationsMetadata"`
	ServiceMetadataURL []ServiceMetadataURL `xml:"ServiceMetadataURL"`
}

type ServiceIdentification struct {
	XMLName		xml.Name `xml:"ows:ServiceIdentification"`
	Title string `xml:"ows:Title"`
	ServiceType   string `xml:"ows:ServiceType"`
	ServiceTypeVersion   string `xml:"ows:ServiceTypeVersion"`
}

type OperationsMetadata struct {
	XMLName		xml.Name `xml:"ows:OperationsMetadata"`
	Operations []Operation `xml:"ows:Operation"`
}
type Operation struct {
	XMLName		xml.Name `xml:"ows:Operation"`
	Name string `xml:"name,attr"`
	DCP  struct {
		HTTP struct {
			Get [] struct {
				Href string `xml:"xlink:href,attr"`
				Contraint struct {
					Name string `xml:"name,attr"`
				} `xml:"ows:Constraint"`
			} `xml:"ows:Get"`
		} `xml:"ows:HTTP"`
	} `xml:"ows:DCP"`
}

type ServiceMetadataURL struct {
	XMLName xml.Name `xml:"ServiceMetadataURL"`
	Xlink string 			`xml:"xlink:href,attr"`
}

func (ts *Tileset) wmtsHandler(w http.ResponseWriter, r *http.Request) {
	db := ts.db
	// imgFormat := db.TileFormatString()
	metadata, err := db.ReadMetadata()
	name, _ := metadata["name"].(string)

	v := &Capabilities { 
		Xmlns:	"http://www.opengis.net/wmts/1.0",
		Ows:		"http://www.opengis.net/ows/1.1",
		Xlink:	"http://www.w3.org/1999/xlink",
		Xsi:		"http://www.w3.org/2001/XMLSchema-instance",
		Gml:		"http://www.opengis.net/gml",
		SchemaLocation:	"http://www.opengis.net/wmts/1.0",
		Version: "1.0.0",
	}

	v.ServiceIdentification = append(v.ServiceIdentification, ServiceIdentification{
		Title: name,
		ServiceType: "OGC WMTS",
		ServiceTypeVersion: "1.0.0",
	})

	operations := make([]Operation, 2)
	operations[0].Name = "GetCapabilities"
	// operations[0] = append(operations[0], )
	// 	DCP: {
	// 		// HTTP: {
	// 		// 	Get: []string{
	// 		// 		Href: "test",
	// 		// 	},
	// 		// 	{
	// 		// 		Href: "test2",
	// 		// 	},
	// 		// },
	// 	},
	// }
	operations[1].Name = "GetTile"

	v.OperationsMetadata = append(v.OperationsMetadata, OperationsMetadata{
		Operations: operations,
	})

	v.ServiceMetadataURL = append(v.ServiceMetadataURL, ServiceMetadataURL{
		Xlink: "test",
	})

	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)
	doc.CreateProcInst("xml-stylesheet", `type="text/xsl" href="style.xsl"`)

	people := doc.CreateElement("People")
	people.CreateComment("These are all known people")

	jon := people.CreateElement("Person")
	jon.CreateAttr("name", "Jon")

	sally := people.CreateElement("Person")
	sally.CreateAttr("name", "Sally")

	doc.Indent(2)

	fmt.Println(fmt.Sprintf("%T", doc))
	fmt.Println(fmt.Sprintf("%T", v))

	str, err := doc.WriteToBytes()
	// res, err := xml.MarshalIndent(doc, "  ", "    ")

  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

	w.Header().Set("Content-Type", "application/xml")
	
	w.Write(str)
}