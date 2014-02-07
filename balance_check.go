package mscd

import (
	"github.com/mastercoin-MSC/mscutil"
	"fmt"
	"encoding/json"
	"os"
)

type Balance struct {
	Real uint64
	Test uint64
}

type Sheet struct {
	Hash string
	Data Balance
}

func TestBalances(db *mscutil.LDBDatabase) bool {
	file, err := os.Open("/home/dev/compare.json")
	if err != nil {
		fmt.Println("FILE:", err)

		os.Exit(1)
	}

	fileInfo, _ := file.Stat()
	jsonBlob := make([]byte, fileInfo.Size())
	file.Read(jsonBlob)

	var sheets []Sheet
	err = json.Unmarshal(jsonBlob, &sheets)
	if err != nil {
		fmt.Println("JSON:", err)
		os.Exit(1)
	}

	failed := 0
	for _, sheet := range sheets {
		acc := db.GetAccount(sheet.Hash)
		if sheet.Data.Real != acc[1] {
			fmt.Printf("Failed cmp for %s %d != %d\n", sheet.Hash, sheet.Data.Real, acc[1])
			failed++
		}
	}

	fmt.Printf("Failed %d (len = %d)\n", failed, len(sheets))


	return true
}
