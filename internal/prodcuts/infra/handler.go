package infra

import (
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"yadwy-backend/internal/common"
	"yadwy-backend/internal/prodcuts/application"
	"yadwy-backend/internal/prodcuts/domain"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

const (
	InvalidRequestBody = "invalid-request-body"
	InvalidProductID   = "invalid-product-id"
)

type ProductHandler struct {
	service *application.ProductService
	logger  *zap.Logger
}

func NewProductHandler(service *application.ProductService, logger *zap.Logger) *ProductHandler {
	return &ProductHandler{
		service: service,
		logger:  logger,
	}
}

func LoadProductsRoutes(b *sqlx.DB, logger *zap.Logger, jwt *common.JWTGenerator) http.Handler {
	ar := chi.NewRouter()
	repo := NewProductRepository(b)
	files, _ := common.NewFileService("/home/nerd/images", "http://localhost:3000/images")
	srv := application.NewProductService(repo, files, logger)
	h := NewProductHandler(srv, logger)

	//ar.Use(common.GetAuthMiddlewareFunc(jwt))
	ar.Get("/{id}", h.GetProduct)
	ar.Post("/", h.CreateProduct)
	ar.Get("/search", h.SearchProducts) // Add search endpoint
	return ar
}

type createProductRequest struct {
	Name        string   `json:"name" validate:"required"`
	Description string   `json:"description"`
	Price       float64  `json:"price" validate:"required,gt=0"`
	CategoryID  string   `json:"category_id" validate:"required"`
	SellerID    int64    `json:"seller_id" validate:"required"`
	Stock       int      `json:"stock" validate:"required,gte=0"`
	IsAvailable bool     `json:"is_available"`
	Labels      []string `json:"labels"`
}

// @Summary Create a new product
// @Description Create a new product with images
// @Tags products
// @Accept multipart/form-data
// @Produce json
// @Param product formData string true "Product data in JSON format"
// @Param main_images formData file false "Main product images"
// @Param thumbnail_images formData file false "Thumbnail images"
// @Param extra_images formData file false "Extra product images"
// @Success 201 {object} domain.Product
// @Failure 400 {object} common.ErrorResponse "Invalid input"
// @Failure 500 {object} common.ErrorResponse "Server error"
// @Router /products [post]
func (h *ProductHandler) CreateProduct(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB max
	if err != nil {
		common.SendError(w, http.StatusBadRequest, InvalidRequestBody, "Failed to parse multipart form")
		return
	}

	// Get product data from form
	productData := r.FormValue("product")
	if productData == "" {
		common.SendError(w, http.StatusBadRequest, InvalidRequestBody, "Product data is required")
		return
	}

	var req createProductRequest
	if err := json.Unmarshal([]byte(productData), &req); err != nil {
		h.logger.Error("Failed to decode product data", zap.Error(err))
		common.SendError(w, http.StatusBadRequest, InvalidRequestBody, "Invalid product data format")
		return
	}

	mainImages := r.MultipartForm.File["main_images"]
	thumbnailImages := r.MultipartForm.File["thumbnail_images"]
	extraImages := r.MultipartForm.File["extra_images"]

	if len(mainImages) == 0 && len(thumbnailImages) == 0 {
		common.SendError(w, http.StatusBadRequest, InvalidRequestBody, "At least one main or thumbnail image is required")
		return
	}

	product := &domain.Product{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		CategoryID:  req.CategoryID,
		SellerID:    req.SellerID,
		Stock:       req.Stock,
		IsAvailable: req.IsAvailable,
		Labels:      req.Labels,
	}

	var allImages []*multipart.FileHeader
	for _, img := range mainImages {
		img.Filename = "main:" + img.Filename
		allImages = append(allImages, img)
	}
	for _, img := range thumbnailImages {
		img.Filename = "thumbnail:" + img.Filename
		allImages = append(allImages, img)
	}
	for _, img := range extraImages {
		img.Filename = "extra:" + img.Filename
		allImages = append(allImages, img)
	}

	err = h.service.CreateProduct(r.Context(), product, allImages)
	if err != nil {
		h.logger.Error("Failed to create product", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, application.FailedToCreateProduct, err.Error())
		return
	}

	if err := common.Encode(w, http.StatusCreated, product); err != nil {
		h.logger.Error("Failed to encode product", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, "failed-to-encode-product", err.Error())
		return
	}
}

// @Summary Get a product by ID
// @Description Get detailed information about a specific product
// @Tags products
// @Produce json
// @Param id path integer true "Product ID"
// @Success 200 {object} domain.Product
// @Failure 400 {object} common.ErrorResponse "Invalid product ID"
// @Failure 404 {object} common.ErrorResponse "Product not found"
// @Failure 500 {object} common.ErrorResponse "Server error"
// @Router /products/{id} [get]
func (h *ProductHandler) GetProduct(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		common.SendError(w, http.StatusBadRequest, InvalidProductID, "Invalid product ID")
		return
	}

	product, err := h.service.GetProduct(r.Context(), id)
	if err != nil {
		h.logger.Error("Failed to get product", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, application.FailedToRetrieveProduct, err.Error())
		return
	}

	if err := common.Encode(w, http.StatusOK, product); err != nil {
		h.logger.Error("Failed to encode product", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, "failed-to-encode-product", err.Error())
		return
	}
}

// @Summary Search products
// @Description Search products with various filters
// @Tags products
// @Produce json
// @Param query query string false "Search query"
// @Param category_id query string false "Category ID"
// @Param min_price query number false "Minimum price"
// @Param max_price query number false "Maximum price"
// @Param seller_id query integer false "Seller ID"
// @Param available query boolean false "Product availability"
// @Param labels query string false "Comma-separated list of labels"
// @Param sort_by query string false "Sort field (name, price, created_at)"
// @Param sort_dir query string false "Sort direction (asc, desc)"
// @Param limit query integer false "Number of items to return (default: 10)"
// @Param offset query integer false "Number of items to skip (default: 0)"
// @Success 200 {array} domain.Product
// @Failure 400 {object} common.ErrorResponse "Invalid parameters"
// @Failure 500 {object} common.ErrorResponse "Server error"
// @Router /products/search [get]
func (h *ProductHandler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// Build search parameters
	params := domain.SearchParams{
		Query:      query.Get("query"),
		CategoryID: query.Get("category_id"),
		Limit:      10, // Default limit
		Offset:     0,  // Default offset
		SortBy:     query.Get("sort_by"),
		SortDir:    query.Get("sort_dir"),
	}

	if limitStr := query.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err == nil && limit > 0 {
			params.Limit = limit
		}
	}

	if offsetStr := query.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err == nil && offset >= 0 {
			params.Offset = offset
		}
	}

	if minPriceStr := query.Get("min_price"); minPriceStr != "" {
		minPrice, err := strconv.ParseFloat(minPriceStr, 64)
		if err == nil && minPrice >= 0 {
			params.MinPrice = &minPrice
		}
	}

	if maxPriceStr := query.Get("max_price"); maxPriceStr != "" {
		maxPrice, err := strconv.ParseFloat(maxPriceStr, 64)
		if err == nil && maxPrice >= 0 {
			params.MaxPrice = &maxPrice
		}
	}

	if sellerIDStr := query.Get("seller_id"); sellerIDStr != "" {
		sellerID, err := strconv.ParseInt(sellerIDStr, 10, 64)
		if err == nil && sellerID > 0 {
			params.SellerID = &sellerID
		}
	}

	if availableStr := query.Get("available"); availableStr != "" {
		available := availableStr == "true" || availableStr == "1"
		params.Available = &available
	}

	if labelsStr := query.Get("labels"); labelsStr != "" {
		params.Labels = strings.Split(labelsStr, ",")
	}

	result, err := h.service.SearchProducts(r.Context(), params)
	if err != nil {
		h.logger.Error("Failed to search products", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, application.FailedToSearchProducts, err.Error())
		return
	}

	if err := common.Encode(w, http.StatusOK, result); err != nil {
		h.logger.Error("Failed to encode search results", zap.Error(err))
		common.SendError(w, http.StatusInternalServerError, "failed-to-encode-product", err.Error())
		return
	}
}
