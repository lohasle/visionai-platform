import router from './router'
import type { RouteRecordRaw } from 'vue-router'
import { isRelogin } from '@/config/axios/service'
import { getAccessToken, removeToken } from '@/utils/auth'
import { deleteUserCache } from '@/hooks/web/useCache'
import { useTitle } from '@/hooks/web/useTitle'
import { useNProgress } from '@/hooks/web/useNProgress'
import { usePageLoading } from '@/hooks/web/usePageLoading'
import { useDictStoreWithOut } from '@/store/modules/dict'
import { useUserStoreWithOut } from '@/store/modules/user'
import { usePermissionStoreWithOut } from '@/store/modules/permission'
import { parseRouteLocation } from '@/utils/routeParams'

const { start, done } = useNProgress()

const { loadStart, loadDone } = usePageLoading()

// 路由不重定向白名单
const whiteList = [
  '/login',
  '/social-login',
  '/auth-redirect',
  '/bind',
  '/register',
  '/oauthLogin/gitee'
]

// 路由加载前
router.beforeEach(async (to, from, next) => {
  start()
  loadStart()
  if (getAccessToken()) {
    if (to.path === '/login') {
      next({ path: '/' })
    } else {
      const dictStore = useDictStoreWithOut()
      const userStore = useUserStoreWithOut()
      const permissionStore = usePermissionStoreWithOut()
      // 异步加载字典
      // 另外，间接 issue：https://gitee.com/nimbuscode/nimbus-ui-admin-vue3/issues/ID9FLI
      if (!dictStore.getIsSetDict) {
        dictStore.setDictMap().then()
      }
      if (!userStore.getIsSetUser) {
        isRelogin.show = true
        try {
          await userStore.setUserInfoAction()
          // 后端过滤菜单
          await permissionStore.generateRoutes()
          permissionStore.getAddRouters.forEach((route) => {
            router.addRoute(route as unknown as RouteRecordRaw) // 动态添加可访问路由表
          })
          const redirectPath = from.query.redirect
          // 修复跳转时不带参数的问题
          const redirect = typeof redirectPath === 'string' ? redirectPath : to.fullPath
          const redirectLocation = parseRouteLocation(redirect)
          // 首次访问动态路由时，to 可能已经匹配静态 404。不能继续展开 to，
          // 否则会把 404 的 route name 带入下一次导航；应始终从原始 URL 重新解析。
          const nextData = { ...redirectLocation, replace: true }
          next(nextData)
        } catch {
          // 过期或已撤销的本地令牌不能把应用永久卡在启动页。
          removeToken()
          deleteUserCache()
          userStore.resetState()
          next(`/login?redirect=${encodeURIComponent(to.fullPath)}`)
        } finally {
          isRelogin.show = false
        }
      } else {
        next()
      }
    }
  } else {
    if (whiteList.indexOf(to.path) !== -1) {
      next()
    } else {
      next(`/login?redirect=${encodeURIComponent(to.fullPath)}`) // 否则全部重定向到登录页
    }
  }
})

router.afterEach((to) => {
  useTitle(to?.meta?.title as string)
  done() // 结束Progress
  loadDone()
})
