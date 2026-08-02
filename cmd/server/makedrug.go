package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/pyq0109/mirgo/internal/log"
	"github.com/pyq0109/mirgo/internal/netserver"
	"github.com/pyq0109/mirgo/internal/protocol"
)



type DrugRecipe struct {
	Product   string         `json:"product"`
	Materials []DrugMaterial `json:"materials"`
	GoldCost  int            `json:"goldCost"`
}

type DrugMaterial struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

var drugRecipes map[string]*DrugRecipe // key: product item name

// LoadDrugRecipes 从 make_items.jsonc 加载制药配方。
func LoadDrugRecipes(configDir string) {
	drugRecipes = make(map[string]*DrugRecipe)
	path := filepath.Join(configDir, "items", "make_items.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		log.Logf(log.LevelInfo, "MakeDrug", "no recipe file at %s, using gold-only mode", path)
		return
	}
	clean := stripJSONCComments(string(data))
	var raw struct {
		Recipes []DrugRecipe `json:"recipes"`
	}
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		log.Logf(log.LevelWarn, "MakeDrug", "failed to parse %s: %v", path, err)
		return
	}
	for i := range raw.Recipes {
		r := &raw.Recipes[i]
		if r.GoldCost <= 0 {
			r.GoldCost = 500
		}
		drugRecipes[r.Product] = r
	}
	log.Logf(log.LevelInfo, "MakeDrug", "loaded %d drug recipes from %s", len(drugRecipes), path)
}

func (p *PlayObject) HandleMakeDrugItem(msg SendMessage, server *netserver.TCPServer) {
	if p.CurrentNpc == nil || !p.CurrentNpc.CanMakeDrug {
		return
	}
	if p.ItemDB == nil {
		return
	}

	itemIdx := int(msg.Param1)
	def := p.ItemDB.GetByIdx(itemIdx)
	if def == nil {
		p.sendMakeDrugFail(server)
		return
	}

	if len(p.ItemList) >= p.Engine.Config.GetMaxBagSlots() {
		p.sendMakeDrugFail(server)
		return
	}

	recipe := drugRecipes[def.Name]
	if recipe == nil {
		// 无配方：金币模式
		drugPrice := p.Engine.Config.GetDrugBasePrice()
		if p.Gold < drugPrice {
			p.sendMakeDrugFail(server)
			return
		}
		p.Gold -= drugPrice
		p.GiveItem(itemIdx)
		p.SendBagItemsFull(server)
		goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
		server.Send(p.Session.ID, goldResp, "")
		p.sendMakeDrugSuccess(server)
		return
	}

	// 有配方：检查材料
	if p.Gold < recipe.GoldCost {
		p.sendMakeDrugFail(server)
		return
	}
	for _, mat := range recipe.Materials {
		if p.countBagItem(mat.Name) < mat.Count {
			p.sendMakeDrugFail(server)
			return
		}
	}
	// 消耗材料和金币
	for _, mat := range recipe.Materials {
		p.removeBagItems(mat.Name, mat.Count)
	}
	p.Gold -= recipe.GoldCost
	p.GiveItem(itemIdx)
	p.SendBagItemsFull(server)
	goldResp := protocol.MakeDefaultMsg(protocol.SMGoldChanged, int32(p.Gold), 0, 0, 0)
	server.Send(p.Session.ID, goldResp, "")
	p.sendMakeDrugSuccess(server)
}

func (p *PlayObject) countBagItem(name string) int {
	if p.ItemDB == nil {
		return 0
	}
	count := 0
	for _, item := range p.ItemList {
		def := p.ItemDB.GetByIdx(int(item.WIndex))
		if def != nil && def.Name == name {
			count++
		}
	}
	return count
}

func (p *PlayObject) removeBagItems(name string, count int) {
	if p.ItemDB == nil {
		return
	}
	removed := 0
	kept := p.ItemList[:0]
	for _, item := range p.ItemList {
		if removed < count {
			def := p.ItemDB.GetByIdx(int(item.WIndex))
			if def != nil && def.Name == name {
				removed++
				continue
			}
		}
		kept = append(kept, item)
	}
	p.ItemList = kept
}

func (p *PlayObject) sendMakeDrugSuccess(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMakeDrugSuccess, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}

func (p *PlayObject) sendMakeDrugFail(server *netserver.TCPServer) {
	resp := protocol.MakeDefaultMsg(protocol.SMMakeDrugFail, 0, 0, 0, 0)
	server.Send(p.Session.ID, resp, "")
}
