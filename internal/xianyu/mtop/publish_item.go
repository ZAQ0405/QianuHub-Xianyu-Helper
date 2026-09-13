package mtop

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// publishItemOnce 组装闲鱼商品发布请求并解析平台发布结果。
func (c *ClientImpl) publishItemOnce(ctx context.Context, cookiesStr string, req PublishItemRequest, images, specImages []uploadedImage, category map[string]any) (*PublishItemResult, error) {
	// imagePayloads 保存上传图片在闲鱼发布协议中的结构。
	imagePayloads := make([]any, 0, len(images))
	// index 表示当前普通图片在发布请求中的顺序。
	// image 表示当前已上传的普通图片。
	for index, image := range images {
		imagePayloads = append(imagePayloads, publishImagePayload(image, index == 0))
	}
	// cat 保存平台最终使用的类目信息。
	cat := mapFromAny(category["categoryPredictResult"])
	// data 保存发布接口要求的业务字段。
	data := map[string]any{
		"freebies": false, "itemTypeStr": "b", "quantity": strconv.Itoa(req.Quantity), "simpleItem": "true", "imageInfoDOList": imagePayloads,
		"itemTextDTO": map[string]any{"desc": req.Description, "title": req.Title, "titleDescSeparate": false}, "itemLabelExtList": publishLabels(category), "itemPriceDTO": publishPriceDTO(req),
		"userRightsProtocols": []any{map[string]any{"enable": false, "serviceCode": "SKILL_PLAY_NO_MIND"}}, "itemPostFeeDTO": postageDTO(req), "defaultPrice": false,
		"itemCatDTO": map[string]any{"catId": mtopString(cat["catId"]), "catName": mtopString(cat["catName"]), "channelCatId": mtopString(cat["channelCatId"]), "tbCatId": mtopString(cat["tbCatId"])},
		"uniqueCode": strconv.FormatInt(time.Now().UnixMicro(), 10), "sourceId": "pcMainPublish", "bizcode": "pcMainPublish", "publishScene": "pcMainPublish",
	}
	if len(req.Specs) > 0 {
		data["itemProperties"] = publishPropertiesPayload(req.Specs, specImages)
		data["itemSkuList"] = publishSKUListPayload(req.SKUs)
		// propertyImages 保存规格值图片上传后的官方引用列表。
		propertyImages := publishPropertyImageList(req.Specs, specImages)
		if len(propertyImages) > 0 {
			data["propertyImageList"] = propertyImages
		}
	}
	if req.Location != nil {
		if !validPublishLocation(*req.Location) {
			return nil, errors.New("发货地信息不完整，请重新定位并选择")
		}
		data["itemAddrDTO"] = map[string]any{"area": req.Location.Area, "city": req.Location.City, "divisionId": req.Location.DivisionID,
			// 闲鱼网页版发布页使用 latitude,longitude 的顺序。
			"gps": fmt.Sprintf("%s,%s", strconv.FormatFloat(req.Location.Latitude, 'f', -1, 64), strconv.FormatFloat(req.Location.Longitude, 'f', -1, 64)), "poiId": req.Location.POIID, "poiName": req.Location.POIName, "prov": req.Location.Province}
	} else if !req.Virtual {
		return nil, errors.New("发布实物商品前必须选择发货地")
	}
	// decoded、updated、err 保存平台响应、Cookie 更新和传输错误。
	decoded, updated, err := c.callMTop(ctx, cookiesStr, PublishItemAPI, "mtop.idle.pc.idleitem.publish", "1.0", "a21ybx.publish.0.0", "a21ybx.home.sidebar.1.46413da6EPl7v5", "46413da6EPl7v5", data)
	if err != nil {
		if isPublishTokenFailure(err) {
			return nil, &PublishError{Code: PublishErrorTokenExpired, Body: err.Error()}
		}
		return nil, err
	}
	// ret 保存平台发布接口返回的业务状态。
	ret := retFromDecoded(decoded)
	if !hasMTopSuccess(ret) {
		// classifiedPublishErr 保存发布域能够从无错误码文本中识别出的专用错误。
		classifiedPublishErr := classifyPublishError(ret, decoded)
		// publishErr 保存专用发布错误分类。
		var publishErr *PublishError
		if errors.As(classifiedPublishErr, &publishErr) && publishErr.Code != PublishErrorUnknown {
			return nil, classifiedPublishErr
		}
		// failure 保存最终发布接口的统一平台失败分类。
		failure := c.mtopResponseFailure("mtop.idle.pc.idleitem.publish", http.StatusOK, ret, "平台 ret 未包含 SUCCESS")
		// kind、ok 保存最终发布错误的分类及存在标记。
		if kind, ok := MTopErrorKindOf(failure); ok && kind != MTopErrorBusiness {
			return nil, failure
		}
		return nil, classifiedPublishErr
	}
	// dataMap 保存发布接口返回数据。
	dataMap := mapFromAny(decoded["data"])
	if dataMap == nil {
		dataMap = make(map[string]any)
	}
	// itemID 保存平台返回的新商品标识。
	itemID := findStringDeep(dataMap, "itemId", "item_id", "id", "itemID")
	if itemID == "" {
		itemID = findStringDeep(decoded, "itemId", "item_id", "itemID")
	}
	// result 保存脱离平台 DTO 后的商品发布结果。
	result := &PublishItemResult{ItemID: itemID, Title: req.Title, PriceText: centsText(req.PriceCents), CategoryID: mtopString(cat["catId"]), CategoryName: mtopString(cat["catName"]), ImageURL: images[0].URL, Quantity: req.Quantity, RawData: dataMap, UpdatedCookies: updated}
	if itemID != "" {
		result.ItemURL = "https://www.goofish.com/item?id=" + itemID
	}
	return result, nil
}
