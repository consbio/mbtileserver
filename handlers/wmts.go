package handlers

import (
	"encoding/xml"
	"fmt"
	// "github.com/beevik/etree"
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
	Operation []Operation `xml:"ows:Operation"`
}
type Operation struct {
	XMLName		xml.Name `xml:"ows:Operation"`
	Name string `xml:"name,attr"`
	DCP  struct {
		HTTP struct {
			Get struct {
				Type string `xml:"xlink:type,attr"`
				Href string `xml:"xlink:href,attr"`
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
	operations[1].Name = "GetTile"

	v.OperationsMetadata = append(v.OperationsMetadata, OperationsMetadata{
		Operation: operations,
	})

	v.ServiceMetadataURL = append(v.ServiceMetadataURL, ServiceMetadataURL{
		Xlink: "test",
	})

	fmt.Println(v)

	res, err := xml.MarshalIndent(v, "  ", "    ")
  if err != nil {
    http.Error(w, err.Error(), http.StatusInternalServerError)
    return
  }

	w.Header().Set("Content-Type", "application/xml")
	w.Write(res)
}