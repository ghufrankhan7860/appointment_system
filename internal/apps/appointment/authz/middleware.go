package authz

import (
	"appointment_system/internal/appcontext"
	"appointment_system/internal/apps/appointment/models"
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequirePermission rejects requests whose principal role lacks the permission.
// It must run after middleware.Authenticate, which sets the principal role.
func RequirePermission(permission Permission) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		applicationContext, ok := appcontext.From(ctx.Request.Context())
		if !ok || applicationContext.PrincipalRole == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Unauthorized"})
			return
		}

		if !HasPermission(models.Role(applicationContext.PrincipalRole), permission) {
			ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": "Forbidden"})
			return
		}

		ctx.Next()
	}
}
