/*
 * Copyright (c) 2024 The GoPlus Authors (goplus.org). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unsafe"

	"github.com/goplus/llgo/c"
	"github.com/goplus/llgo/c/cjson"
	"github.com/goplus/llgo/chore/_xtool/llcppsymg/config"
	"github.com/goplus/llgo/chore/_xtool/llcppsymg/parse"
	"github.com/goplus/llgo/chore/llcppg/types"
	"github.com/goplus/llgo/cpp/llvm"
	"github.com/goplus/llgo/xtool/nm"
)

func main() {
	cfgFile := "llcppg.cfg"
	if len(os.Args) > 1 {
		cfgFile = os.Args[1]
	}

	var data []byte
	var err error
	if cfgFile == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(cfgFile)
	}
	check(err)

	conf, err := config.GetConf(data)
	check(err)
	defer conf.Delete()

	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse config file:", cfgFile)
	}
	symbols, err := parseDylibSymbols(conf.Libs)

	check(err)

	filepaths := generateHeaderFilePath(conf.CFlags, conf.Include)
	astInfos, err := parse.ParseHeaderFile(filepaths)
	check(err)

	symbolInfo := getCommonSymbols(symbols, astInfos, conf.TrimPrefixes)

	err = genSymbolTableFile(symbolInfo)
	check(err)
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func parseDylibSymbols(lib string) ([]types.CPPSymbol, error) {
	dylibPath, err := generateDylibPath(lib)
	if err != nil {
		return nil, errors.New("failed to generate dylib path")
	}

	files, err := nm.New("").List(dylibPath)
	if err != nil {
		return nil, errors.New("failed to list symbols in dylib")
	}

	var symbols []types.CPPSymbol

	for _, file := range files {
		for _, sym := range file.Symbols {
			demangleName := decodeSymbolName(sym.Name)
			symbols = append(symbols, types.CPPSymbol{
				Symbol:       sym,
				DemangleName: demangleName,
			})
		}
	}

	return symbols, nil
}

func generateDylibPath(lib string) (string, error) {
	output := lib
	libPath := ""
	libName := ""
	for _, part := range strings.Fields(string(output)) {
		if strings.HasPrefix(part, "-L") {
			libPath = part[2:]
		} else if strings.HasPrefix(part, "-l") {
			libName = part[2:]
		}
	}

	if libPath == "" || libName == "" {
		return "", fmt.Errorf("failed to parse pkg-config output: %s", output)
	}

	dylibPath := filepath.Join(libPath, "lib"+libName+".dylib")
	return dylibPath, nil
}

func decodeSymbolName(symbolName string) string {
	if symbolName == "" {
		return ""
	}

	demangled := llvm.ItaniumDemangle(symbolName, true)
	if demangled == nil {
		return symbolName
	}
	defer c.Free(unsafe.Pointer(demangled))

	demangleName := c.GoString(demangled)
	if demangleName == "" {
		return symbolName
	}

	decodedName := strings.TrimSpace(demangleName)
	decodedName = strings.ReplaceAll(decodedName,
		"std::__1::basic_string<char, std::__1::char_traits<char>, std::__1::allocator<char> > const",
		"std::string")

	return decodedName
}

func generateHeaderFilePath(cflags string, files []string) []string {
	prefixPath := cflags
	prefixPath = strings.TrimPrefix(prefixPath, "-I")
	var includePaths []string
	for _, file := range files {
		includePaths = append(includePaths, filepath.Join(prefixPath, "/"+file))
	}
	return includePaths
}

func getCommonSymbols(dylibSymbols []types.CPPSymbol, astInfoList []types.ASTInformation, prefix []string) []types.SymbolInfo {
	var commonSymbols []types.SymbolInfo
	functionNameMap := make(map[string]int)

	for _, astInfo := range astInfoList {
		for _, dylibSym := range dylibSymbols {
			if strings.TrimPrefix(dylibSym.Name, "_") == astInfo.Symbol {
				cppName := generateCPPName(astInfo)
				functionNameMap[cppName]++
				symbolInfo := types.SymbolInfo{
					Mangle: strings.TrimPrefix(dylibSym.Name, "_"),
					CPP:    cppName,
					Go:     generateMangle(astInfo, functionNameMap[cppName], prefix),
				}
				commonSymbols = append(commonSymbols, symbolInfo)
				break
			}
		}
	}

	return commonSymbols
}

func generateCPPName(astInfo types.ASTInformation) string {
	cppName := astInfo.Name
	if astInfo.Class != "" {
		cppName = astInfo.Class + "::" + astInfo.Name
	}
	return cppName
}

func generateMangle(astInfo types.ASTInformation, count int, prefixes []string) string {
	astInfo.Class = removePrefix(astInfo.Class, prefixes)
	astInfo.Name = removePrefix(astInfo.Name, prefixes)
	res := ""
	if astInfo.Class != "" {
		if astInfo.Class == astInfo.Name {
			res = "(*" + astInfo.Class + ")." + "Init"
			if count > 1 {
				res += "__" + strconv.Itoa(count-1)
			}
		} else if astInfo.Name == "~"+astInfo.Class {
			res = "(*" + astInfo.Class + ")." + "Dispose"
			if count > 1 {
				res += "__" + strconv.Itoa(count-1)
			}
		} else {
			res = "(*" + astInfo.Class + ")." + astInfo.Name
			if count > 1 {
				res += "__" + strconv.Itoa(count-1)
			}
		}
	} else {
		res = astInfo.Name
		if count > 1 {
			res += "__" + strconv.Itoa(count-1)
		}
	}
	return res
}

func removePrefix(str string, prefixes []string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(str, prefix) {
			return strings.TrimPrefix(str, prefix)
		}
	}
	return str
}

func genSymbolTableFile(symbolInfos []types.SymbolInfo) error {
	// keep open follow code block can run successfully
	for i := range symbolInfos {
		println("symbol", symbolInfos[i].Go)
	}

	fileName := "llcppg.symb.json"
	existingSymbols, err := readExistingSymbolTable(fileName)
	if err != nil {
		return err
	}

	for i := range symbolInfos {
		if existingSymbol, exists := existingSymbols[symbolInfos[i].Mangle]; exists {
			symbolInfos[i].Go = existingSymbol.Go
		}
	}

	root := cjson.Array()
	defer root.Delete()

	for _, symbol := range symbolInfos {
		item := cjson.Object()
		item.SetItem(c.Str("mangle"), cjson.String(c.AllocaCStr(symbol.Mangle)))
		item.SetItem(c.Str("c++"), cjson.String(c.AllocaCStr(symbol.CPP)))
		item.SetItem(c.Str("go"), cjson.String(c.AllocaCStr(symbol.Go)))
		root.AddItem(item)
	}

	cStr := root.Print()
	if cStr == nil {
		return errors.New("symbol table is empty")
	}
	defer c.Free(unsafe.Pointer(cStr))

	data := unsafe.Slice((*byte)(unsafe.Pointer(cStr)), c.Strlen(cStr))

	if err := os.WriteFile(fileName, data, 0644); err != nil {
		return errors.New("failed to write symbol table file")
	}
	return nil
}
func readExistingSymbolTable(fileName string) (map[string]types.SymbolInfo, error) {
	existingSymbols := make(map[string]types.SymbolInfo)

	if _, err := os.Stat(fileName); err != nil {
		return existingSymbols, nil
	}

	data, err := os.ReadFile(fileName)
	if err != nil {
		return nil, errors.New("failed to read symbol table file")
	}

	parsedJSON := cjson.ParseBytes(data)
	if parsedJSON == nil {
		return nil, errors.New("failed to parse JSON")
	}

	arraySize := parsedJSON.GetArraySize()

	for i := 0; i < int(arraySize); i++ {
		item := parsedJSON.GetArrayItem(c.Int(i))
		if item == nil {
			continue
		}
		symbol := types.SymbolInfo{
			Mangle: config.GetStringItem(item, "mangle", ""),
			CPP:    config.GetStringItem(item, "c++", ""),
			Go:     config.GetStringItem(item, "go", ""),
		}
		existingSymbols[symbol.Mangle] = symbol
	}

	return existingSymbols, nil
}
